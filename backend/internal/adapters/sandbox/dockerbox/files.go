package dockerbox

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/sandbox"
)

// missingPathExit is the exit code the delete wrapper uses to say "no such
// path", chosen outside the range shells and coreutils use for their own
// failures.
const missingPathExit = 66

// WriteFile copies data into the sandbox through the Docker copy API. The
// archive carries the container user's uid/gid and CopyUIDGID tells the daemon
// to honour them, otherwise everything would land owned by root and the agent
// could not touch its own files.
//
// AGENTS.md is bind-mounted read-only, so a write to it fails at the
// filesystem, which is the point: the approval-gated edit path is host-side.
func (s *sandboxHandle) WriteFile(ctx context.Context, name string, data []byte, mode fs.FileMode) error {
	abs, err := sandbox.ResolvePath(name)
	if err != nil {
		return fmt.Errorf("sandbox: write file: %w", err)
	}
	id, finish, err := s.rt.beginOperation(ctx, s.ref)
	if err != nil {
		return err
	}
	defer finish()
	s.containerID = id

	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}

	parent := path.Dir(abs)
	base := path.Base(abs)
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return fmt.Errorf("sandbox: write %q: randomize temporary name: %w", name, err)
	}
	tmp := ".protean-write-" + hex.EncodeToString(token)
	command := fmt.Sprintf(`
parent=%s
/bin/mkdir -p -- "$parent" || exit 70
exec 3< "$parent" || exit 71
actual=$(/usr/bin/readlink -f /proc/self/fd/3) || exit 72
[ "$actual" = "$parent" ] || exit 73
tmp=/proc/self/fd/3/%s
trap '/bin/rm -f -- "$tmp"' EXIT HUP INT TERM
/bin/cat > "$tmp" || exit 74
/bin/chmod %04o "$tmp" || exit 75
/bin/mv -fT -- "$tmp" /proc/self/fd/3/%s || exit 76
trap - EXIT HUP INT TERM
`, shellQuote(parent), tmp, perm, shellQuote(base))
	res, err := s.execInput(ctx, id, command, data, 30*time.Second)
	if err != nil {
		return fmt.Errorf("sandbox: write %q: %w", name, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("sandbox: write %q: secure writer exit %d: %s", name, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}

// ReadFile pulls one file out of the sandbox as a tar stream.
func (s *sandboxHandle) ReadFile(ctx context.Context, name string) ([]byte, error) {
	abs, err := sandbox.ResolvePath(name)
	if err != nil {
		return nil, fmt.Errorf("sandbox: read file: %w", err)
	}
	id, finish, err := s.rt.beginOperation(ctx, s.ref)
	if err != nil {
		return nil, err
	}
	defer finish()
	s.containerID = id

	rc, _, err := s.rt.api.CopyFromContainer(ctx, id, abs)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, fmt.Errorf("sandbox: read %q: %w", name, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("sandbox: read %q: %w", name, err)
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("sandbox: read %q: %w", name, fs.ErrNotExist)
		}
		if err != nil {
			return nil, fmt.Errorf("sandbox: read %q: %w", name, err)
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
			if hdr.Size > MaxFileBytes {
				return nil, fmt.Errorf("sandbox: read %q: file is %d bytes, over the %d byte limit",
					name, hdr.Size, int64(MaxFileBytes))
			}
			data, err := io.ReadAll(io.LimitReader(tr, MaxFileBytes+1))
			if err != nil {
				return nil, fmt.Errorf("sandbox: read %q: %w", name, err)
			}
			if len(data) > MaxFileBytes {
				return nil, fmt.Errorf("sandbox: read %q: file is over the %d byte limit", name, int64(MaxFileBytes))
			}
			return data, nil
		case tar.TypeDir:
			return nil, fmt.Errorf("sandbox: read %q: path is a directory", name)
		}
	}
}

// DeleteFile removes a path, recursively when it is a directory. The path guard
// already ruled out the workspace root, `.protean`, and anything outside the
// workspace, so the recursive remove cannot reach further than one project
// subtree.
func (s *sandboxHandle) DeleteFile(ctx context.Context, name string) error {
	abs, err := sandbox.ResolvePath(name)
	if err != nil {
		return fmt.Errorf("sandbox: delete file: %w", err)
	}

	quoted := shellQuote(abs)
	command := fmt.Sprintf("if [ ! -e %s ] && [ ! -L %s ]; then exit %d; fi; rm -rf -- %s",
		quoted, quoted, missingPathExit, quoted)

	res, err := s.Exec(ctx, ports.ExecSpec{Command: command, Timeout: 30 * time.Second})
	if err != nil {
		return fmt.Errorf("sandbox: delete %q: %w", name, err)
	}
	switch res.ExitCode {
	case 0:
		return nil
	case missingPathExit:
		return fmt.Errorf("sandbox: delete %q: %w", name, fs.ErrNotExist)
	default:
		return fmt.Errorf("sandbox: delete %q: exit %d: %s", name, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
}

// containerUser resolves the uid/gid the container runs as, once per sandbox.
// Asking the container beats trusting the image's User field, which may be a
// name rather than a number.
func (r *Runtime) containerUser(ctx context.Context, s *sandboxHandle) (int, int, error) {
	st := r.stateFor(s.ref)

	r.mu.Lock()
	if st.idsKnown {
		uid, gid := st.uid, st.gid
		r.mu.Unlock()
		return uid, gid, nil
	}
	r.mu.Unlock()

	res, err := s.Exec(ctx, ports.ExecSpec{Command: "id -u; id -g", Timeout: 15 * time.Second})
	if err != nil {
		return 0, 0, fmt.Errorf("sandbox: resolve container user: %w", err)
	}
	fields := strings.Fields(res.Stdout)
	if res.ExitCode != 0 || len(fields) < 2 {
		return 0, 0, fmt.Errorf("sandbox: resolve container user: exit %d, output %q", res.ExitCode, res.Stdout)
	}
	uid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("sandbox: resolve container uid: %w", err)
	}
	gid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("sandbox: resolve container gid: %w", err)
	}

	r.mu.Lock()
	st.uid, st.gid, st.idsKnown = uid, gid, true
	r.mu.Unlock()
	return uid, gid, nil
}

// tarFile builds an archive with a directory entry for every missing parent and
// then the file itself, all owned by uid/gid.
func tarFile(rel string, data []byte, perm fs.FileMode, uid, gid int, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	dir := path.Dir(rel)
	if dir != "." {
		prefix := ""
		for _, seg := range strings.Split(dir, "/") {
			prefix = path.Join(prefix, seg)
			hdr := &tar.Header{
				Name:     prefix + "/",
				Typeflag: tar.TypeDir,
				Mode:     0o755,
				Uid:      uid,
				Gid:      gid,
				ModTime:  now,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return nil, fmt.Errorf("sandbox: build archive: %w", err)
			}
		}
	}

	hdr := &tar.Header{
		Name:     rel,
		Typeflag: tar.TypeReg,
		Mode:     int64(perm),
		Size:     int64(len(data)),
		Uid:      uid,
		Gid:      gid,
		ModTime:  now,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("sandbox: build archive: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return nil, fmt.Errorf("sandbox: build archive: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("sandbox: build archive: %w", err)
	}
	return buf.Bytes(), nil
}

// shellQuote wraps s in single quotes so the shell treats it as one literal
// word, whatever it contains.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
