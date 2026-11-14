package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	DefaultStopTimeout  = 10
	HealthCheckInterval = 1 * time.Second
)

type DockerManager struct {
	client *client.Client
}

func NewDockerManager() (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerManager{client: cli}, nil
}

func (d *DockerManager) PullImage(ctx context.Context, imageName string) error {
	reader, err := d.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	// Read the output to ensure pull completes
	_, err = io.Copy(io.Discard, reader)
	return err
}

func (d *DockerManager) CreateContainer(ctx context.Context, config *ContainerConfig) (string, error) {
	// Port binding — loopback only: dev databases must not be reachable from
	// other machines.
	portBinding := nat.PortBinding{
		HostIP:   "127.0.0.1",
		HostPort: fmt.Sprintf("%d", config.Port),
	}

	containerPort, err := nat.NewPort("tcp", config.ContainerPort)
	if err != nil {
		return "", err
	}

	// Container config
	containerConfig := &container.Config{
		Image: config.Image,
		Env:   config.Env,
		ExposedPorts: nat.PortSet{
			containerPort: struct{}{},
		},
	}

	// Host config with volume mounts
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			containerPort: []nat.PortBinding{portBinding},
		},
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	// Add volume mounts if specified
	if config.VolumeName != "" && config.VolumePath != "" {
		hostConfig.Mounts = []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: config.VolumeName,
				Target: config.VolumePath,
			},
		}
	}

	// Create container
	resp, err := d.client.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		config.Name,
	)
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (d *DockerManager) StartContainer(ctx context.Context, containerID string) error {
	return d.client.ContainerStart(ctx, containerID, container.StartOptions{})
}

func (d *DockerManager) StopContainer(ctx context.Context, containerID string) error {
	timeout := DefaultStopTimeout
	return d.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func (d *DockerManager) RemoveContainer(ctx context.Context, containerID string) error {
	return d.client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

func (d *DockerManager) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	inspect, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}
	return inspect.State.Running, nil
}

// GetContainerLogs retrieves logs from a container
func (d *DockerManager) GetContainerLogs(ctx context.Context, containerID string, tail string, follow bool) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
		Timestamps: true,
	}
	return d.client.ContainerLogs(ctx, containerID, options)
}

// ExecResult holds the captured output and exit status of a finished exec.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Exec runs cmd in the container, optionally feeding stdin to it, and returns
// the captured output. err reports transport failures only; check ExitCode for
// the command's own status.
func (d *DockerManager) Exec(ctx context.Context, containerID string, cmd []string, stdin string) (*ExecResult, error) {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	if stdin != "" {
		execConfig.AttachStdin = true
	}

	execID, err := d.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create exec: %w", err)
	}

	resp, err := d.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach exec: %w", err)
	}
	defer resp.Close()

	// Write stdin concurrently: a large dump can fill the pipe while the
	// command's output fills ours, which would deadlock on sequential I/O.
	stdinErr := make(chan error, 1)
	if stdin != "" {
		go func() {
			_, err := resp.Conn.Write([]byte(stdin))
			if err == nil {
				resp.CloseWrite()
			}
			stdinErr <- err
		}()
	}

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return nil, fmt.Errorf("failed to read exec output: %w", err)
	}
	if stdin != "" {
		if err := <-stdinErr; err != nil {
			return nil, fmt.Errorf("failed to write stdin: %w", err)
		}
	}

	inspect, err := d.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect exec: %w", err)
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: inspect.ExitCode,
	}, nil
}

// ExecCommand runs cmd in the container and fails if it exits non-zero.
func (d *DockerManager) ExecCommand(ctx context.Context, containerID string, cmd []string) (string, error) {
	res, err := d.Exec(ctx, containerID, cmd, "")
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("command failed (exit %d): %s", res.ExitCode, res.Stderr)
	}
	return res.Stdout, nil
}

// VolumeExists reports whether a named volume exists.
func (d *DockerManager) VolumeExists(ctx context.Context, name string) (bool, error) {
	_, err := d.client.VolumeInspect(ctx, name)
	if client.IsErrNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RemoveVolume deletes a named volume.
func (d *DockerManager) RemoveVolume(ctx context.Context, name string) error {
	return d.client.VolumeRemove(ctx, name, false)
}

type ContainerConfig struct {
	Name          string
	Image         string
	Port          int
	ContainerPort string
	Env           []string
	VolumeName    string
	VolumePath    string
}
