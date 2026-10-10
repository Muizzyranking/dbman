// Package docker is the seam between dbman and the Docker Engine. The
// current Client shells out to the docker CLI; a future SDK implementation
// can satisfy the same interface.
package docker

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Spec describes how to `docker run` a database container.
type Spec struct {
	Image         string
	Name          string // container name
	HostPort      int
	ContainerPort int
	Env           map[string]string
	Volumes       []string
	Network       string
	ExtraArgs     []string // placed before image (docker run flags)
	Command       []string // placed after image (container CMD override)
}

// Status is a lightweight view of `docker ps` state.
type Status struct {
	Running bool
	State   string // running | exited | created | notfound
}

// Client is the seam between dbman and Docker.
// Current impl shells out to the `docker` CLI; a future SDK impl
// can satisfy the same interface.
type Client interface {
	ContainerStatus(container string) Status
	Run(spec Spec) error
	Start(container string) error
	Stop(container string) error
	Restart(container string) error
	Remove(container string, volumes bool) error
	Logs(container string, follow bool, tail string) error
	LogsCapture(container string, tail string) (string, error)
	ImageExists(image string) bool
	Pull(image string) error
	ExecInteractive(container string, cmd ...string) error
	ExecCapture(container string, cmd ...string) (string, error)
	PortInUseBy(port int) string // container name using host port, "" if none
}

type CLIClient struct{}

func NewCLIClient() *CLIClient { return &CLIClient{} }

func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func (c *CLIClient) ContainerStatus(container string) Status {
	out, err := runCapture("docker", "ps", "-a", "--filter", "name=^/"+container+"$", "--format", "{{.State}}")
	if err != nil {
		return Status{Running: false, State: "unknown"}
	}
	s := strings.TrimSpace(out)
	if s == "" {
		return Status{Running: false, State: "notfound"}
	}
	first := strings.ToLower(strings.Fields(s)[0])
	return Status{Running: first == "running", State: first}
}

func (c *CLIClient) Run(spec Spec) error {
	args := []string{"run", "-d", "--name", spec.Name}
	args = append(args, "-p", fmt.Sprintf("%d:%d", spec.HostPort, spec.ContainerPort))
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for _, vol := range spec.Volumes {
		args = append(args, "-v", vol)
	}
	if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	args = append(args, spec.ExtraArgs...)
	args = append(args, spec.Image)
	args = append(args, spec.Command...)
	// NOTE: output is captured, never written to os.Stdout. Writing to the
	// real stdout corrupts the Bubble Tea alt-screen (ghost rows that look
	// like duplicated databases). Callers print their own messages.
	_, err := runCapture("docker", args...)
	return err
}

func (c *CLIClient) Start(container string) error {
	_, err := runCapture("docker", "start", container)
	return err
}

func (c *CLIClient) Stop(container string) error {
	_, err := runCapture("docker", "stop", container)
	return err
}

func (c *CLIClient) Restart(container string) error {
	_, err := runCapture("docker", "restart", container)
	return err
}

func (c *CLIClient) Remove(container string, volumes bool) error {
	args := []string{"rm", "-f"}
	if volumes {
		args = append(args, "-v")
	}
	args = append(args, container)
	_, err := runCapture("docker", args...)
	return err
}

// Logs attaches to docker logs. With follow=true this blocks until Ctrl-C.
func (c *CLIClient) Logs(container string, follow bool, tail string) error {
	args := []string{"logs"}
	if tail != "" {
		args = append(args, "--tail", tail)
	}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, container)
	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ExecInteractive runs `docker exec -it ...` attached to the terminal.
// Use for DB consoles (psql/redis-cli/...) and container shells.
func (c *CLIClient) ExecInteractive(container string, cmd ...string) error {
	args := append([]string{"exec", "-it", container}, cmd...)
	proc := exec.Command("docker", args...)
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	return proc.Run()
}

func (c *CLIClient) ExecCapture(container string, cmd ...string) (string, error) {
	args := append([]string{"exec", container}, cmd...)
	return runCapture("docker", args...)
}

func (c *CLIClient) LogsCapture(container string, tail string) (string, error) {
	if tail == "" {
		tail = "200"
	}
	return runCapture("docker", "logs", "--tail", tail, container)
}

// ImageExists reports whether the image is already present locally.
func (c *CLIClient) ImageExists(image string) bool {
	cmd := exec.Command("docker", "image", "inspect", image)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// Pull downloads the image. Output is captured; callers show progress.
func (c *CLIClient) Pull(image string) error {
	_, err := runCapture("docker", "pull", image)
	return err
}

func (c *CLIClient) PortInUseBy(port int) string {
	out, err := runCapture("docker", "ps", "--format", "{{.Names}} {{.Ports}}")
	if err != nil {
		return ""
	}
	needle := fmt.Sprintf("%d->", port)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			if f := strings.Fields(line); len(f) > 0 {
				return f[0]
			}
		}
	}
	return ""
}
