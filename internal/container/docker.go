package container

import (
	"bufio"
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
	// Port binding
	portBinding := nat.PortBinding{
		HostIP:   "0.0.0.0",
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

func (d *DockerManager) WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for container to be healthy")
		case <-ticker.C:
			running, err := d.IsContainerRunning(ctx, containerID)
			if err != nil {
				continue
			}
			if running {
				return nil
			}
		}
	}
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

// ExecCommand executes a command in a running container
func (d *DockerManager) ExecCommand(ctx context.Context, containerID string, cmd []string) (string, error) {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	execID, err := d.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}

	resp, err := d.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach exec: %w", err)
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return "", fmt.Errorf("failed to read exec output: %w", err)
	}

	if stderr.Len() > 0 {
		return "", fmt.Errorf("exec error: %s", stderr.String())
	}

	return stdout.String(), nil
}

// StreamLogs streams container logs line by line to a channel
func (d *DockerManager) StreamLogs(ctx context.Context, containerID string, lines chan<- string) error {
	logs, err := d.GetContainerLogs(ctx, containerID, "100", true)
	if err != nil {
		return err
	}
	defer logs.Close()

	scanner := bufio.NewScanner(logs)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case lines <- scanner.Text():
		}
	}
	return scanner.Err()
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
