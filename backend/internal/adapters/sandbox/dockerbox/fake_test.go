package dockerbox

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// notFound is shaped like a daemon 404 so client.IsErrNotFound recognises it.
func notFound(what string) error {
	return fmt.Errorf("no such %s: %w", what, cerrdefs.ErrNotFound)
}

// createCall records one ContainerCreate so tests can assert the whole request.
type createCall struct {
	name    string
	config  *container.Config
	host    *container.HostConfig
	network *network.NetworkingConfig
}

// execCall records one exec, its options, and the scripted response.
type execCall struct {
	containerID string
	opts        container.ExecOptions
}

// execScript is one scripted exec outcome.
type execScript struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

// fakeDocker is a scripted stand-in for the Docker Engine API.
type fakeDocker struct {
	mu sync.Mutex

	imageMissing bool

	// containers maps name to its inspect response; a missing key is a 404.
	containers map[string]container.InspectResponse
	// list is what ContainerList returns.
	list []container.Summary

	creates  []createCall
	started  []string
	stopped  []string
	removed  []string
	execs    []execCall
	copiedTo []copyCall
	// copyFrom is the tar stream ReadFile receives, keyed by source path.
	copyFrom map[string][]byte

	// execScripts are consumed in order; the last one repeats.
	execScripts []execScript
	execIndex   int

	createErr error
	startErr  error
}

type copyCall struct {
	containerID string
	dstPath     string
	content     []byte
	opts        container.CopyToContainerOptions
}

var _ dockerAPI = (*fakeDocker)(nil)

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		containers: map[string]container.InspectResponse{},
		copyFrom:   map[string][]byte{},
	}
}

// setContainer registers a container under its name, running or not.
func (f *fakeDocker) setContainer(name, id string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.containers[name] = container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    id,
			Name:  "/" + name,
			State: &container.State{Running: running},
		},
	}
}

// script queues exec outcomes, consumed in order.
func (f *fakeDocker) script(outcomes ...execScript) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execScripts = append(f.execScripts, outcomes...)
}

func (f *fakeDocker) snapshotExecs() []execCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]execCall, len(f.execs))
	copy(out, f.execs)
	return out
}

// snapshotStopped and snapshotStarted are the guarded readers the tests that
// run the reaper need, because that goroutine writes these slices.
func (f *fakeDocker) snapshotStopped() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stopped...)
}

func (f *fakeDocker) snapshotStarted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func (f *fakeDocker) Ping(context.Context) (types.Ping, error) {
	return types.Ping{APIVersion: "1.55"}, nil
}

func (f *fakeDocker) ImageInspect(_ context.Context, imageID string, _ ...client.ImageInspectOption) (image.InspectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.imageMissing {
		return image.InspectResponse{}, notFound("image " + imageID)
	}
	return image.InspectResponse{ID: "sha256:fake"}, nil
}

func (f *fakeDocker) ContainerList(context.Context, container.ListOptions) ([]container.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, nil
}

func (f *fakeDocker) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.containers[id]; ok {
		return c, nil
	}
	return container.InspectResponse{}, notFound("container " + id)
}

func (f *fakeDocker) ContainerCreate(_ context.Context, config *container.Config, hostConfig *container.HostConfig, net *network.NetworkingConfig, _ *ocispec.Platform, name string) (container.CreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return container.CreateResponse{}, f.createErr
	}
	id := "container-" + name
	f.creates = append(f.creates, createCall{name: name, config: config, host: hostConfig, network: net})
	f.containers[name] = container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    id,
			Name:  "/" + name,
			State: &container.State{Running: false},
		},
	}
	return container.CreateResponse{ID: id}, nil
}

func (f *fakeDocker) ContainerStart(_ context.Context, id string, _ container.StartOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, id)
	for name, c := range f.containers {
		if c.ID == id {
			// A fresh State avoids aliasing the pointer a caller already holds.
			c.State = &container.State{Running: true}
			f.containers[name] = c
		}
	}
	return nil
}

func (f *fakeDocker) ContainerStop(_ context.Context, id string, _ container.StopOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.containers[id]
	if !ok {
		return notFound("container " + id)
	}
	c.State = &container.State{Running: false}
	f.containers[id] = c
	f.stopped = append(f.stopped, id)
	return nil
}

func (f *fakeDocker) ContainerRemove(_ context.Context, id string, _ container.RemoveOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.containers[id]; !ok {
		return notFound("container " + id)
	}
	delete(f.containers, id)
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeDocker) ContainerExecCreate(_ context.Context, id string, opts container.ExecOptions) (container.ExecCreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execs = append(f.execs, execCall{containerID: id, opts: opts})
	return container.ExecCreateResponse{ID: fmt.Sprintf("exec-%d", len(f.execs)-1)}, nil
}

// ContainerExecAttach serves the scripted output over a real socket pair, in
// the daemon's multiplexed framing, so the adapter's demultiplexing is
// exercised rather than stubbed out.
func (f *fakeDocker) ContainerExecAttach(_ context.Context, execID string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
	script := f.scriptFor(execID)

	server, clientConn := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = server.Write(muxFrame(1, script.stdout))
		_, _ = server.Write(muxFrame(2, script.stderr))
	}()
	return types.NewHijackedResponse(clientConn, "application/vnd.docker.multiplexed-stream"), nil
}

func (f *fakeDocker) ContainerExecInspect(_ context.Context, execID string) (container.ExecInspect, error) {
	script := f.scriptFor(execID)
	return container.ExecInspect{ExecID: execID, Running: false, ExitCode: script.exitCode}, nil
}

// scriptFor returns the outcome for the nth exec, repeating the last entry.
func (f *fakeDocker) scriptFor(execID string) execScript {
	f.mu.Lock()
	defer f.mu.Unlock()

	var n int
	if _, err := fmt.Sscanf(execID, "exec-%d", &n); err != nil {
		return execScript{}
	}
	if len(f.execScripts) == 0 {
		return execScript{}
	}
	if n >= len(f.execScripts) {
		n = len(f.execScripts) - 1
	}
	return f.execScripts[n]
}

func (f *fakeDocker) CopyToContainer(_ context.Context, id, dst string, content io.Reader, opts container.CopyToContainerOptions) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.copiedTo = append(f.copiedTo, copyCall{containerID: id, dstPath: dst, content: data, opts: opts})
	return nil
}

func (f *fakeDocker) CopyFromContainer(_ context.Context, _, src string) (io.ReadCloser, container.PathStat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tarball, ok := f.copyFrom[src]
	if !ok {
		return nil, container.PathStat{}, notFound("path " + src)
	}
	return io.NopCloser(bytes.NewReader(tarball)), container.PathStat{Name: src}, nil
}

func (f *fakeDocker) Close() error { return nil }

// muxFrame wraps payload in the daemon's 8-byte stream header.
func muxFrame(stream byte, payload []byte) []byte {
	if len(payload) == 0 {
		return nil
	}
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	return append(header, payload...)
}
