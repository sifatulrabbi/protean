package dockerbox

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// dockerAPI is the slice of the Docker Engine API this adapter uses. Declaring
// it narrowly keeps the unit tests free of a daemon: *client.Client satisfies
// it, and so does the fake in the tests.
type dockerAPI interface {
	Ping(ctx context.Context) (types.Ping, error)
	ImageInspect(ctx context.Context, imageID string, opts ...client.ImageInspectOption) (image.InspectResponse, error)

	ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, opts container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, opts container.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, opts container.RemoveOptions) error

	ContainerExecCreate(ctx context.Context, containerID string, opts container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, opts container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error)

	CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, opts container.CopyToContainerOptions) error
	CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, container.PathStat, error)

	Close() error
}

var _ dockerAPI = (*client.Client)(nil)
