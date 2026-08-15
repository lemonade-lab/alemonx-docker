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
var containerNameReference = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
var networkReference = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
var volumeReference = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
var wordReference = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
var portSpecReference = regexp.MustCompile(`^[0-9A-Za-z\[\]:./-]{1,128}$`)
var envLineReference = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(=.*)?$`)
var volumeMountReference = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}:(/[^:]*)(:(ro|rw))?$`)
var labelLineReference = regexp.MustCompile(`^[A-Za-z0-9_.-]+(=.*)?$`)

var validRestartPolicies = map[string]bool{"no": true, "always": true, "on-failure": true, "unless-stopped": true}

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

// dockerInputSink lets tests observe stdin payloads without echoing secrets
// into task output. It is a no-op in production.
var dockerInputSink = func(string) {}

func runDockerInput(dir, input string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := execCommand(ctx, "docker", args...)
	command.Dir = dir
	command.Stdin = strings.NewReader(input)
	dockerInputSink(input)
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
	var args []string
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
	output, err := composeInProject(id, args...)
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

func imageTag(source, target string) (actionResult, error) {
	source, target = strings.TrimSpace(source), strings.TrimSpace(target)
	if !imageReference.MatchString(source) || !imageReference.MatchString(target) {
		return actionResult{}, errors.New("镜像引用无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "image", "tag", source, target)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("标记镜像失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已将 " + source + " 标记为 " + target + "。\n" + clipOutput(output)}, nil
}

func imagePush(reference string) (actionResult, error) {
	reference = strings.TrimSpace(reference)
	if !imageReference.MatchString(reference) {
		return actionResult{}, errors.New("镜像引用无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "image", "push", reference)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("推送镜像失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已推送镜像 " + reference + "。\n" + clipOutput(output)}, nil
}

func imagePrune(all bool) (actionResult, error) {
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	args := []string{"image", "prune", "-f"}
	if all {
		args = append(args, "-a")
	}
	output, err := runDocker("", args...)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("清理镜像失败：%s", shortDockerError(err, output))
	}
	label := "悬空镜像"
	if all {
		label = "未使用镜像"
	}
	return actionResult{Output: "✓ 已清理" + label + "。\n" + clipOutput(output)}, nil
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
	return containerLabels(labels)["com.docker.compose.project"]
}

func containerLabels(raw string) map[string]string {
	labels := map[string]string{}
	for _, label := range strings.Split(raw, ",") {
		if key, value, found := strings.Cut(label, "="); found {
			labels[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return labels
}

func parseJSONOutput(output string) any {
	var value any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &value); err == nil {
		return value
	}
	return strings.TrimSpace(output)
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

func containerBatch(verb string, ids []string) (actionResult, error) {
	labels := map[string]string{"start": "启动", "stop": "停止", "restart": "重启"}
	if _, ok := labels[verb]; !ok {
		return actionResult{}, errors.New("不支持的批量操作")
	}
	if len(ids) == 0 || len(ids) > 100 {
		return actionResult{}, errors.New("容器 ID 数量无效")
	}
	for _, id := range ids {
		if !containerReference.MatchString(id) {
			return actionResult{}, errors.New("容器 ID 无效")
		}
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	args := append([]string{"container", verb}, ids...)
	output, err := runDocker("", args...)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("容器%s失败：%s", verb, shortDockerError(err, output))
	}
	return actionResult{Output: fmt.Sprintf("✓ 已%s %d 个容器。\n%s", labels[verb], len(ids), clipOutput(output))}, nil
}

func containerInspect(id string) (actionResult, error) {
	if !containerReference.MatchString(strings.TrimSpace(id)) {
		return actionResult{}, errors.New("容器 ID 无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "container", "inspect", id)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("读取容器详情失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已读取容器 " + id + " 详情", Data: parseJSONOutput(output)}, nil
}

// containerCreate maps the browser-submitted form to a whitelisted
// `docker run -d` command. Environment values and labels never appear in task
// output or errors; volumes are restricted to named-volume mounts.
func containerCreate(params map[string]string) (actionResult, error) {
	image := strings.TrimSpace(params["image"])
	if !imageReference.MatchString(image) {
		return actionResult{}, errors.New("镜像引用无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	args := []string{"run", "-d"}
	displayName := ""
	if name := strings.TrimSpace(params["name"]); name != "" {
		if !containerNameReference.MatchString(name) {
			return actionResult{}, errors.New("容器名称无效（仅允许字母、数字、点、下划线与短横线）")
		}
		displayName = name
		args = append(args, "--name", name)
	}
	if restart := strings.TrimSpace(params["restart"]); restart != "" {
		if !validRestartPolicies[restart] {
			return actionResult{}, errors.New("重启策略无效（仅支持 no/always/on-failure/unless-stopped）")
		}
		args = append(args, "--restart", restart)
	}
	if network := strings.TrimSpace(params["network"]); network != "" {
		if !networkReference.MatchString(network) {
			return actionResult{}, errors.New("网络名称无效")
		}
		args = append(args, "--network", network)
	}
	ports, err := parseFormLines(params["ports"], 50, "端口", false, validatePortSpec)
	if err != nil {
		return actionResult{}, err
	}
	for _, port := range ports {
		args = append(args, "--publish", port)
	}
	envs, err := parseFormLines(params["env"], 100, "环境变量", true, validateEnvLine)
	if err != nil {
		return actionResult{}, err
	}
	envArgs := []string{}
	for _, env := range envs {
		envArgs = append(envArgs, "--env", env)
	}
	args = append(args, envArgs...)
	volumes, err := parseFormLines(params["volumes"], 50, "卷", false, validateVolumeMount)
	if err != nil {
		return actionResult{}, err
	}
	for _, volume := range volumes {
		args = append(args, "--volume", volume)
	}
	labels, err := parseFormLines(params["labels"], 50, "标签", false, validateLabelLine)
	if err != nil {
		return actionResult{}, err
	}
	for _, label := range labels {
		args = append(args, "--label", label)
	}
	command, err := splitCommandWords(params["command"])
	if err != nil {
		return actionResult{}, err
	}
	args = append(args, image)
	args = append(args, command...)
	output, err := runDocker("", args...)
	secrets := envArgs
	if err != nil {
		return actionResult{Output: redactSecrets(output, secrets)}, fmt.Errorf("创建容器失败：%s", redactSecrets(shortDockerError(err, output), secrets))
	}
	id := strings.TrimSpace(output)
	if displayName == "" && id != "" {
		displayName = "ID " + shortID(id)
	}
	if displayName == "" {
		displayName = "（ID 未知）"
	}
	return actionResult{Output: "✓ 已创建容器 " + displayName + "（环境变量内容不会回显）。", Data: map[string]string{"containerID": id}}, nil
}

// parseFormLines splits a newline-separated form field, enforcing limits and
// field validation. redact controls whether the offending line is echoed in
// the validation error (kept false for environment values).
func parseFormLines(raw string, limit int, field string, redact bool, validate func(string) error) ([]string, error) {
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.ContainsRune(line, '\x00') {
			return nil, fmt.Errorf("%s包含非法字符", field)
		}
		if len(lines) >= limit {
			return nil, fmt.Errorf("%s数量超过限制（最多 %d 行）", field, limit)
		}
		if err := validate(line); err != nil {
			if redact {
				return nil, fmt.Errorf("%s无效：%s", field, err)
			}
			return nil, fmt.Errorf("%s无效（%s）：%s", field, line, err)
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func validatePortSpec(line string) error {
	if strings.HasPrefix(line, "-") {
		return errors.New("不能以 - 开头")
	}
	if !portSpecReference.MatchString(line) {
		return errors.New("格式应为 [IP:]宿主端口:容器端口[/协议] 或 容器端口[/协议]")
	}
	return nil
}

func validateEnvLine(line string) error {
	if strings.HasPrefix(line, "-") {
		return errors.New("不能以 - 开头")
	}
	if !envLineReference.MatchString(line) {
		return errors.New("格式应为 KEY=value 或 KEY")
	}
	return nil
}

func validateVolumeMount(line string) error {
	if !volumeMountReference.MatchString(line) {
		return errors.New("仅支持命名卷，格式为 卷名:容器路径[:ro|rw]，不支持宿主路径")
	}
	return nil
}

func validateLabelLine(line string) error {
	if strings.HasPrefix(line, "-") {
		return errors.New("不能以 - 开头")
	}
	if !labelLineReference.MatchString(line) {
		return errors.New("格式应为 KEY=value 或 KEY")
	}
	return nil
}

// splitCommandWords splits a command into arguments, honouring single and
// double quotes, without any shell interpretation. Unclosed quotes and
// control characters are rejected.
func splitCommandWords(raw string) ([]string, error) {
	words := []string{}
	var builder strings.Builder
	inSingle, inDouble, started := false, false, false
	for _, r := range raw {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			started = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			started = true
		case (r == ' ' || r == '\t' || r == '\n' || r == '\r') && !inSingle && !inDouble:
			if started {
				words = append(words, builder.String())
				builder.Reset()
				started = false
			}
		default:
			if r < 0x20 {
				return nil, errors.New("命令包含非法控制字符")
			}
			builder.WriteRune(r)
			started = true
		}
	}
	if inSingle || inDouble {
		return nil, errors.New("命令包含未闭合的引号")
	}
	if started {
		words = append(words, builder.String())
	}
	if len(words) > 100 {
		return nil, errors.New("命令参数过多（最多 100 个）")
	}
	return words, nil
}

func redactSecrets(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "<redacted>")
		}
	}
	return text
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

type DockerStat struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CPUPerc  string `json:"cpuPerc"`
	MemUsage string `json:"memUsage"`
	MemPerc  string `json:"memPerc"`
	NetIO    string `json:"netIO"`
	BlockIO  string `json:"blockIO"`
	PIDs     string `json:"pids"`
}

func containerStats(id string) (DockerStat, error) {
	if !containerReference.MatchString(strings.TrimSpace(id)) {
		return DockerStat{}, errors.New("容器 ID 无效")
	}
	if err := requireDocker(); err != nil {
		return DockerStat{}, err
	}
	output, err := runDocker("", "stats", "--no-stream", "--format", "{{json .}}", id)
	if err != nil {
		return DockerStat{}, fmt.Errorf("读取容器统计失败：%s", shortDockerError(err, output))
	}
	var raw struct {
		BlockIO   string
		CPUPerc   string
		Container string
		ID        string
		MemPerc   string
		MemUsage  string
		Name      string
		NetIO     string
		PIDs      string
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &raw); err != nil {
		return DockerStat{}, fmt.Errorf("解析容器统计失败：%s", shortDockerError(err, output))
	}
	idField := raw.ID
	if idField == "" {
		idField = raw.Container
	}
	return DockerStat{ID: idField, Name: raw.Name, CPUPerc: raw.CPUPerc, MemUsage: raw.MemUsage, MemPerc: raw.MemPerc, NetIO: raw.NetIO, BlockIO: raw.BlockIO, PIDs: raw.PIDs}, nil
}

func containerLogsSince(id, since string, lines int) (actionResult, error) {
	if !containerReference.MatchString(strings.TrimSpace(id)) {
		return actionResult{}, errors.New("容器 ID 无效")
	}
	since = strings.TrimSpace(since)
	if _, err := time.Parse(time.RFC3339Nano, since); err != nil {
		return actionResult{}, errors.New("时间戳无效（需要 RFC3339 格式，例如 2026-08-15T12:00:00Z）")
	}
	if lines < 1 {
		lines = 1
	}
	if lines > 5000 {
		lines = 5000
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "container", "logs", "--since", since, "--tail", strconv.Itoa(lines), "--timestamps", "--no-color", id)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("读取容器日志失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: clipOutput(output)}, nil
}

const maxActionOutput = 4000

func clipOutput(output string) string {
	runes := []rune(strings.TrimSpace(output))
	if len(runes) <= maxActionOutput {
		return string(runes)
	}
	return string(runes[:maxActionOutput]) + "\n…（输出过长，已截断）"
}
