package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var execCommand = exec.CommandContext
var imageReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,254}$`)
var containerReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,255}$`)

type DockerCheck struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Detail    string `json:"detail,omitempty"`
}
type DockerStatus struct {
	CLI      DockerCheck `json:"cli"`
	Compose  DockerCheck `json:"compose"`
	Daemon   DockerCheck `json:"daemon"`
	Platform string      `json:"platform"`
}
type DockerImage struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}
type DockerContainer struct {
	ID        string          `json:"id"`
	Names     string          `json:"names"`
	Image     string          `json:"image"`
	State     string          `json:"state"`
	Status    string          `json:"status"`
	Ports     string          `json:"ports"`
	Labels    string          `json:"labels"`
	Project   string          `json:"project"`
	Published []PublishedPort `json:"published"`
}

// PublishedPort is one parsed host->container port mapping from docker ps.
type PublishedPort struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

// DockerPortRow flattens published ports across containers for the ports view.
type DockerPortRow struct {
	ContainerID   string `json:"containerID"`
	Names         string `json:"names"`
	Project       string `json:"project"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

func dockerStatus() DockerStatus {
	cliOut, cliErr := runDocker("", "version", "--format", "{{.Client.Version}}")
	cli := dockerCheck(cliOut, cliErr)
	composeOut, composeErr := runDocker("", "compose", "version", "--short")
	daemonOut, daemonErr := runDocker("", "info", "--format", "{{.ServerVersion}}")
	return DockerStatus{CLI: cli, Compose: dockerCheck(composeOut, composeErr), Daemon: dockerCheck(daemonOut, daemonErr), Platform: runtimePlatform()}
}

func dockerCheck(output string, err error) DockerCheck {
	if err == nil {
		return DockerCheck{Available: true, Version: strings.TrimSpace(output)}
	}
	return DockerCheck{Detail: shortDockerError(err, output)}
}

func runtimePlatform() string { return goos + "/" + goarch }

// Variables keep runtime information independently replaceable in tests.
var goos, goarch = runtime.GOOS, runtime.GOARCH

func runDocker(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := execCommand(ctx, "docker", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if ctx.Err() != nil {
		return text, errors.New("Docker 命令超时")
	}
	if err != nil {
		return text, fmt.Errorf("%w", err)
	}
	return text, nil
}

func shortDockerError(err error, output string) string {
	text := strings.TrimSpace(output)
	if text == "" {
		text = err.Error()
	}
	if len([]rune(text)) > 280 {
		text = string([]rune(text)[:280]) + "…"
	}
	return text
}

func requireDocker() error {
	status := dockerStatus()
	if !status.CLI.Available {
		return errors.New("未检测到 Docker CLI，请先安装 Docker")
	}
	if !status.Daemon.Available {
		return errors.New("Docker 守护进程未运行或当前账户没有访问权限")
	}
	return nil
}

func composeAction(action, id string) (actionResult, error) {
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	dir, err := projectPath(id)
	if err != nil {
		return actionResult{}, err
	}
	if _, err := readMeta(dir); err != nil {
		return actionResult{}, err
	}
	args := []string{"compose", "-f", composePath(dir)}
	label := ""
	switch action {
	case "compose-up":
		args = append(args, "up", "-d")
		label = "已启动项目"
	case "compose-stop":
		args = append(args, "stop")
		label = "已停止项目"
	case "compose-restart":
		args = append(args, "restart")
		label = "已重启项目"
	case "compose-down":
		args = append(args, "down")
		label = "已关闭项目（未删除卷或镜像）"
	}
	output, err := runDocker(dir, args...)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("Compose 操作失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ " + label + "。\n" + clipOutput(output)}, nil
}

func dockerImages() ([]DockerImage, error) {
	if err := requireDocker(); err != nil {
		return nil, err
	}
	output, err := runDocker("", "image", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("读取镜像失败：%s", shortDockerError(err, output))
	}
	items := []DockerImage{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw struct {
			ID           string
			Repository   string
			Tag          string
			Size         string
			CreatedSince string
		}
		if json.Unmarshal([]byte(line), &raw) == nil {
			items = append(items, DockerImage{raw.ID, raw.Repository, raw.Tag, raw.Size, raw.CreatedSince})
		}
	}
	return items, nil
}

func imagePull(image string) (actionResult, error) {
	if !imageReference.MatchString(strings.TrimSpace(image)) {
		return actionResult{}, errors.New("镜像引用无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "image", "pull", image)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("拉取镜像失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已拉取镜像 " + image + "。\n" + clipOutput(output)}, nil
}

func imageRemove(image string) (actionResult, error) {
	if !imageReference.MatchString(strings.TrimSpace(image)) {
		return actionResult{}, errors.New("镜像引用无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "image", "rm", image)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("删除镜像失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已删除镜像 " + image + "。\n" + clipOutput(output)}, nil
}

func dockerContainers() ([]DockerContainer, error) {
	if err := requireDocker(); err != nil {
		return nil, err
	}
	output, err := runDocker("", "container", "ls", "-a", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("读取容器失败：%s", shortDockerError(err, output))
	}
	items := []DockerContainer{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw struct {
			ID     string
			Names  string
			Image  string
			State  string
			Status string
			Ports  string
			Labels string
		}
		if json.Unmarshal([]byte(line), &raw) == nil {
			items = append(items, DockerContainer{raw.ID, raw.Names, raw.Image, raw.State, raw.Status, raw.Ports, raw.Labels, composeProject(raw.Labels), parsePublishedPorts(raw.Ports)})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.EqualFold(items[i].State, "running") && !strings.EqualFold(items[j].State, "running")
	})
	return items, nil
}

// parsePublishedPorts parses the docker ps ports column. Entries without a
// host mapping (e.g. "443/tcp") are internal-only and skipped; IPv6 bracket
// forms and port ranges keep their first host port.
func parsePublishedPorts(raw string) []PublishedPort {
	ports := []PublishedPort{}
	seen := map[string]bool{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "->", 2)
		if len(parts) != 2 {
			continue
		}
		hostPort := parsePortNumber(parts[0])
		containerPort, protocol := parseContainerPort(parts[1])
		if hostPort == 0 || containerPort == 0 {
			continue
		}
		key := strconv.Itoa(hostPort) + "/" + protocol + "/" + strconv.Itoa(containerPort)
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, PublishedPort{HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol})
	}
	return ports
}

func parsePortNumber(part string) int {
	part = strings.TrimSpace(part)
	if colon := strings.LastIndexByte(part, ':'); colon >= 0 {
		part = part[colon+1:]
	}
	part = strings.Trim(part, "[]")
	if dash := strings.IndexByte(part, '-'); dash >= 0 {
		part = part[:dash]
	}
	value, err := strconv.Atoi(part)
	if err != nil || value < 1 || value > 65535 {
		return 0
	}
	return value
}

func parseContainerPort(part string) (int, string) {
	protocol := "tcp"
	part = strings.TrimSpace(part)
	if slash := strings.LastIndexByte(part, '/'); slash >= 0 {
		protocol = strings.ToLower(strings.TrimSpace(part[slash+1:]))
		part = part[:slash]
	}
	if dash := strings.IndexByte(part, '-'); dash >= 0 {
		part = part[:dash]
	}
	value, err := strconv.Atoi(strings.TrimSpace(part))
	if err != nil || value < 1 || value > 65535 {
		return 0, protocol
	}
	return value, protocol
}

// dockerPorts flattens every published port across containers so the web UI
// can show which host ports Docker currently occupies.
func dockerPorts() ([]DockerPortRow, error) {
	items, err := dockerContainers()
	if err != nil {
		return nil, err
	}
	rows := []DockerPortRow{}
	for _, item := range items {
		for _, port := range item.Published {
			rows = append(rows, DockerPortRow{ContainerID: item.ID, Names: item.Names, Project: item.Project, HostPort: port.HostPort, ContainerPort: port.ContainerPort, Protocol: port.Protocol})
		}
	}
	return rows, nil
}

func composeProject(labels string) string {
	for _, label := range strings.Split(labels, ",") {
		if key, value, found := strings.Cut(label, "="); found && strings.TrimSpace(key) == "com.docker.compose.project" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containerLogs(id string) (actionResult, error) {
	if !containerReference.MatchString(strings.TrimSpace(id)) {
		return actionResult{}, errors.New("容器 ID 无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "container", "logs", "--tail", "200", id)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("读取容器日志失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: clipOutput(output)}, nil
}

func containerAction(action, id string) (actionResult, error) {
	if !containerReference.MatchString(strings.TrimSpace(id)) {
		return actionResult{}, errors.New("容器 ID 无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	verb := strings.TrimPrefix(action, "container-")
	output, err := runDocker("", "container", verb, id)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("容器%s失败：%s", verb, shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 容器已" + map[string]string{"start": "启动", "stop": "停止", "restart": "重启"}[verb] + "。\n" + clipOutput(output)}, nil
}

const maxActionOutput = 4000

func clipOutput(output string) string {
	runes := []rune(strings.TrimSpace(output))
	if len(runes) <= maxActionOutput {
		return string(runes)
	}
	return string(runes[:maxActionOutput]) + "\n…（输出过长，已截断）"
}
