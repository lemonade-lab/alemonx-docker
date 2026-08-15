package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerHelper(t *testing.T) {
	if os.Getenv("ALX_DOCKER_HELPER") != "1" {
		return
	}
	if os.Getenv("ALX_DOCKER_HELPER_FAIL_CLI") == "1" {
		fmt.Fprintln(os.Stderr, "docker: command not found")
		os.Exit(1)
	}
	args := os.Args
	for index, value := range args {
		if value == "--" && index+1 < len(args) {
			args = args[index+1:]
			break
		}
	}
	joined := strings.Join(args, " ")
	switch {
	case os.Getenv("ALX_DOCKER_HELPER_FAIL_COMPOSE") == "1" && joined == "compose version --short":
		fmt.Fprintln(os.Stderr, "docker compose: plugin not found")
		os.Exit(1)
	case os.Getenv("ALX_DOCKER_HELPER_FAIL_DAEMON") == "1" && strings.HasPrefix(joined, "info "):
		fmt.Fprintln(os.Stderr, "Cannot connect to the Docker daemon")
		os.Exit(1)
	case os.Getenv("ALX_DOCKER_HELPER_FAIL_NEXT") == "1" &&
		joined != "version --format {{.Client.Version}}" &&
		joined != "compose version --short" &&
		joined != "info --format {{.ServerVersion}}":
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(1)
	case os.Getenv("ALX_DOCKER_HELPER_IMAGES") == "1" && strings.Contains(joined, "image ls"):
		_, _ = os.Stdout.WriteString(`{"ID":"sha256:abc","Repository":"nginx","Tag":"latest","Size":"50MB","CreatedSince":"2 weeks ago"}` + "\n")
	case os.Getenv("ALX_DOCKER_HELPER_CONTAINERS") == "1" && strings.Contains(joined, "container ls"):
		_, _ = os.Stdout.WriteString(`{"ID":"abc123","Names":"web-1","Image":"nginx:latest","State":"running","Status":"Up 2 minutes","Ports":"0.0.0.0:8080->80/tcp, 443/tcp","Labels":"com.docker.compose.project=myapp,com.docker.compose.service=web,com.docker.compose.project.working_dir=/srv/myapp,com.docker.compose.project.config_files=/srv/myapp/compose.yml"}` + "\n")
	case os.Getenv("ALX_DOCKER_HELPER_NETWORKS") == "1" && strings.Contains(joined, "network ls"):
		_, _ = os.Stdout.WriteString(`{"Name":"bridge","Driver":"bridge","Scope":"local"}` + "\n")
		_, _ = os.Stdout.WriteString(`{"Name":"demo-net","Driver":"bridge","Scope":"local"}` + "\n")
	case os.Getenv("ALX_DOCKER_HELPER_VOLUMES") == "1" && strings.Contains(joined, "volume ls"):
		_, _ = os.Stdout.WriteString(`{"Name":"vol-data","Driver":"local","Scope":"local","Mountpoint":"/var/lib/docker/volumes/vol-data/_data"}` + "\n")
	case os.Getenv("ALX_DOCKER_HELPER_STATS") == "1" && strings.Contains(joined, "stats --no-stream"):
		_, _ = os.Stdout.WriteString(`{"BlockIO":"1.2kB / 0B","CPUPerc":"0.05%","Container":"abc123","ID":"abc123","MemPerc":"0.10%","MemUsage":"5MiB / 8GiB","Name":"web-1","NetIO":"1.5kB / 2kB","PIDs":"3"}` + "\n")
	case os.Getenv("ALX_DOCKER_HELPER_COMPOSE_PS") == "1" && strings.HasSuffix(joined, "ps -a --format json"):
		_, _ = os.Stdout.WriteString(`{"ID":"abc123","Name":"status-web-1","Service":"web","State":"running","Status":"Up 2 minutes","Image":"nginx:latest"}` + "\n")
	case joined == "version --format {{.Client.Version}}":
		_, _ = os.Stdout.WriteString("27.0.0")
	case joined == "compose version --short":
		_, _ = os.Stdout.WriteString("v2.29.0")
	case joined == "info --format {{.ServerVersion}}":
		_, _ = os.Stdout.WriteString("27.0.0")
	default:
		if file := os.Getenv("ALX_DOCKER_HELPER_ARGS_FILE"); file != "" {
			_ = os.WriteFile(file, []byte(strings.Join(args, "|")), 0600)
		}
		if os.Getenv("ALX_DOCKER_HELPER_CWD") == "1" {
			cwd, _ := os.Getwd()
			_, _ = os.Stdout.WriteString("cwd=" + cwd + "\n")
		}
		_, _ = os.Stdout.WriteString(strings.Join(args, "|"))
	}
	os.Exit(0)
}

func fakeDocker(ctx context.Context, _ string, args ...string) *exec.Cmd {
	return fakeDockerWith(ctx, nil, args...)
}

func fakeDockerWith(ctx context.Context, env []string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestDockerHelper", "--")
	command.Args = append(command.Args, args...)
	command.Env = append(os.Environ(), "ALX_DOCKER_HELPER=1")
	command.Env = append(command.Env, env...)
	return command
}

func withDocker(t *testing.T, env ...string) {
	t.Helper()
	original := execCommand
	execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return fakeDockerWith(ctx, env, args...)
	}
	t.Cleanup(func() { execCommand = original })
}

func TestDockerStatusHealthy(t *testing.T) {
	withDocker(t)
	status := dockerStatus()
	if !status.CLI.Available || !status.Compose.Available || !status.Daemon.Available {
		t.Fatalf("healthy status: %+v", status)
	}
	if status.CLI.Version != "27.0.0" || status.Compose.Version != "v2.29.0" || status.Daemon.Version != "27.0.0" {
		t.Fatalf("unexpected versions: %+v", status)
	}
}

func TestDockerStatusMissingCLI(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_CLI=1")
	status := dockerStatus()
	if status.CLI.Available || status.Compose.Available || status.Daemon.Available {
		t.Fatalf("CLI missing must fail every check: %+v", status)
	}
	if !strings.Contains(status.CLI.Detail, "not found") {
		t.Fatalf("detail should surface docker error: %+v", status.CLI)
	}
}

func TestDockerStatusDaemonDown(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_DAEMON=1")
	status := dockerStatus()
	if !status.CLI.Available || !status.Compose.Available || status.Daemon.Available {
		t.Fatalf("only daemon should be down: %+v", status)
	}
	if !strings.Contains(status.Daemon.Detail, "daemon") {
		t.Fatalf("daemon detail should explain the failure: %+v", status.Daemon)
	}
}

func TestDockerStatusComposeMissing(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_COMPOSE=1")
	status := dockerStatus()
	if !status.CLI.Available || status.Compose.Available || !status.Daemon.Available {
		t.Fatalf("only compose plugin should be missing: %+v", status)
	}
}

func TestComposeLifecycleUsesExactCommandsAndProjectDir(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	withDocker(t, "ALX_DOCKER_HELPER_CWD=1")
	dir, err := projectPath(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for action, verb := range map[string]string{
		"compose-up":      "up|-d",
		"compose-stop":    "stop",
		"compose-restart": "restart",
		"compose-down":    "down",
	} {
		result, err := composeAction(action, project.ID)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !strings.Contains(result.Output, "compose|-f|"+composePath(dir)+"|"+verb) {
			t.Fatalf("%s used unexpected command: %q", action, result.Output)
		}
		if !strings.Contains(result.Output, "cwd="+realDir) {
			t.Fatalf("%s did not run in project dir %q: %q", action, realDir, result.Output)
		}
	}
}

func TestComposeDownUsesNoVolumeOrImageDeletionFlags(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	withDocker(t)
	result, err := composeAction("compose-down", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "compose|-f|") || !strings.Contains(result.Output, "|down") {
		t.Fatalf("unexpected compose command: %q", result.Output)
	}
	if strings.Contains(result.Output, "|-v") || strings.Contains(result.Output, "--rmi") || strings.Contains(result.Output, "--volumes") {
		t.Fatalf("unsafe compose command: %q", result.Output)
	}
}

func TestComposeActionRequiresManagedProject(t *testing.T) {
	useTemporaryProjectRoot(t)
	withDocker(t)
	if _, err := composeAction("compose-up", "../escape"); err == nil {
		t.Fatal("invalid project ID must fail")
	}
	if _, err := composeAction("compose-up", "missing-project"); err == nil {
		t.Fatal("unknown project must fail")
	}
}

func TestComposeErrorMapsDockerOutput(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Error")
	if err != nil {
		t.Fatal(err)
	}
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_NEXT=1")
	result, err := composeAction("compose-up", project.ID)
	if err == nil || !strings.Contains(err.Error(), "Compose 操作失败") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should map docker failure: %v", err)
	}
	if !strings.Contains(result.Output, "boom") {
		t.Fatalf("raw docker output should be returned: %q", result.Output)
	}
}

func TestImageActionsUseExactCommands(t *testing.T) {
	withDocker(t)
	pulled, err := imagePull("nginx:latest")
	if err != nil || !strings.Contains(pulled.Output, "image|pull|nginx:latest") {
		t.Fatalf("pull = %+v, %v", pulled, err)
	}
	removed, err := imageRemove("nginx:1.27")
	if err != nil || !strings.Contains(removed.Output, "image|rm|nginx:1.27") {
		t.Fatalf("remove = %+v, %v", removed, err)
	}
}

func TestImageActionErrorMapsDockerOutput(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_NEXT=1")
	result, err := imagePull("nginx:latest")
	if err == nil || !strings.Contains(err.Error(), "拉取镜像失败") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should map docker failure: %v", err)
	}
	if !strings.Contains(result.Output, "boom") {
		t.Fatalf("raw docker output should be returned: %q", result.Output)
	}
}

func TestImageListParsesRows(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_IMAGES=1")
	items, err := dockerImages()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Repository != "nginx" || items[0].Tag != "latest" || items[0].Size != "50MB" {
		t.Fatalf("images = %+v", items)
	}
}

func TestContainerActionsUseExactCommands(t *testing.T) {
	withDocker(t)
	for action, verb := range map[string]string{
		"container-start":   "start",
		"container-stop":    "stop",
		"container-restart": "restart",
	} {
		result, err := containerAction(action, "abc123")
		if err != nil || !strings.Contains(result.Output, "container|"+verb+"|abc123") {
			t.Fatalf("%s = %+v, %v", action, result, err)
		}
	}
	logs, err := containerLogs("abc123")
	if err != nil || !strings.Contains(logs.Output, "container|logs|--tail|200|abc123") {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
}

func TestContainerListExtractsComposeProject(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_CONTAINERS=1")
	items, err := dockerContainers()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "abc123" || items[0].Project != "myapp" || items[0].State != "running" {
		t.Fatalf("containers = %+v", items)
	}
	if len(items[0].Published) != 1 || items[0].Published[0].HostPort != 8080 || items[0].Published[0].ContainerPort != 80 || items[0].Published[0].Protocol != "tcp" {
		t.Fatalf("published ports = %+v", items[0].Published)
	}
}

func TestParsePublishedPorts(t *testing.T) {
	ports := parsePublishedPorts("0.0.0.0:8080->80/tcp, [::]:8080->80/tcp, 127.0.0.1:9000->9000/udp, 443/tcp, 0.0.0.0:3000-3005->3000-3005/tcp")
	if len(ports) != 3 {
		t.Fatalf("ports = %+v", ports)
	}
	if ports[0].HostPort != 8080 || ports[0].ContainerPort != 80 || ports[0].Protocol != "tcp" {
		t.Fatalf("first port = %+v", ports[0])
	}
	if ports[1].HostPort != 9000 || ports[1].Protocol != "udp" {
		t.Fatalf("udp port = %+v", ports[1])
	}
	if ports[2].HostPort != 3000 || ports[2].ContainerPort != 3000 {
		t.Fatalf("range port = %+v", ports[2])
	}
}

func TestDockerPortsFlattensContainers(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_CONTAINERS=1")
	rows, err := dockerPorts()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].HostPort != 8080 || rows[0].ContainerID != "abc123" || rows[0].Project != "myapp" {
		t.Fatalf("port rows = %+v", rows)
	}
}

func TestDockerActionsRejectUnsafeReferences(t *testing.T) {
	for _, value := range []string{"", "nginx; rm -rf /", "../nginx", "image name", "--force nginx", "nginx --privileged"} {
		if _, err := imagePull(value); err == nil {
			t.Fatalf("image %q must fail", value)
		}
		if _, err := containerAction("container-start", value); err == nil {
			t.Fatalf("container %q must fail", value)
		}
		if _, err := containerLogs(value); err == nil {
			t.Fatalf("logs %q must fail", value)
		}
	}
}

func TestClipOutput(t *testing.T) {
	short := strings.Repeat("a", 100)
	if clipped := clipOutput(short); clipped != short {
		t.Fatalf("short output changed: %q", clipped)
	}
	long := strings.Repeat("b", maxActionOutput+50)
	clipped := clipOutput(long)
	if !strings.Contains(clipped, "已截断") || len([]rune(clipped)) >= len([]rune(long)) {
		t.Fatalf("long output not truncated: %d runes", len([]rune(clipped)))
	}
}

func TestImageTagPushPruneUseExactCommands(t *testing.T) {
	withDocker(t)
	tagged, err := imageTag("nginx:latest", "registry.example.com/app/nginx:v1")
	if err != nil || !strings.Contains(tagged.Output, "image|tag|nginx:latest|registry.example.com/app/nginx:v1") {
		t.Fatalf("tag = %+v, %v", tagged, err)
	}
	pushed, err := imagePush("registry.example.com/app/nginx:v1")
	if err != nil || !strings.Contains(pushed.Output, "image|push|registry.example.com/app/nginx:v1") {
		t.Fatalf("push = %+v, %v", pushed, err)
	}
	pruned, err := imagePrune(false)
	if err != nil || !strings.Contains(pruned.Output, "image|prune|-f") || strings.Contains(pruned.Output, "|-a") {
		t.Fatalf("prune dangling = %+v, %v", pruned, err)
	}
	all, err := imagePrune(true)
	if err != nil || !strings.Contains(all.Output, "image|prune|-f|-a") {
		t.Fatalf("prune all = %+v, %v", all, err)
	}
}

func TestImageActionsRejectUnsafeReferences(t *testing.T) {
	for _, value := range []string{"", "nginx; rm -rf /", "../nginx", "nginx --force", "image name"} {
		if _, err := imageTag(value, "ok"); err == nil {
			t.Fatalf("tag source %q must fail", value)
		}
		if _, err := imageTag("ok", value); err == nil {
			t.Fatalf("tag target %q must fail", value)
		}
		if _, err := imagePush(value); err == nil {
			t.Fatalf("push %q must fail", value)
		}
	}
}

func TestNetworkActionsUseExactCommands(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_NETWORKS=1")
	items, err := dockerNetworks()
	if err != nil || len(items) != 2 || items[0].Name != "bridge" || items[1].Name != "demo-net" {
		t.Fatalf("networks = %+v, %v", items, err)
	}
	created, err := networkCreate("demo-net", "bridge", "172.20.0.0/16", "172.20.0.1")
	if err != nil || !strings.Contains(created.Output, "network|create|--driver|bridge|--subnet|172.20.0.0/16|--gateway|172.20.0.1|demo-net") {
		t.Fatalf("create = %+v, %v", created, err)
	}
	simple, err := networkCreate("plain-net", "", "", "")
	if err != nil || !strings.Contains(simple.Output, "network|create|plain-net") || strings.Contains(simple.Output, "--driver") {
		t.Fatalf("simple create = %+v, %v", simple, err)
	}
	removed, err := networkRemove("demo-net")
	if err != nil || !strings.Contains(removed.Output, "network|rm|demo-net") {
		t.Fatalf("remove = %+v, %v", removed, err)
	}
	inspected, err := networkInspect("demo-net")
	if err != nil || inspected.Data != "network|inspect|demo-net" {
		t.Fatalf("inspect = %+v, %v", inspected, err)
	}
}

func TestNetworkCreateValidatesInputs(t *testing.T) {
	withDocker(t)
	for _, args := range [][]string{
		{"../bad", "", "", ""},
		{"ok", "bridge; rm -rf /", "", ""},
		{"ok", "", "999.1.1.0/24", ""},
		{"ok", "", "", "not-an-ip"},
	} {
		if _, err := networkCreate(args[0], args[1], args[2], args[3]); err == nil {
			t.Fatalf("networkCreate(%q, %q, %q, %q) must fail", args[0], args[1], args[2], args[3])
		}
	}
}

func TestVolumeActionsUseExactCommands(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_VOLUMES=1")
	items, err := dockerVolumes()
	if err != nil || len(items) != 1 || items[0].Name != "vol-data" || items[0].Mountpoint != "/var/lib/docker/volumes/vol-data/_data" {
		t.Fatalf("volumes = %+v, %v", items, err)
	}
	created, err := volumeCreate("data", "local")
	if err != nil || !strings.Contains(created.Output, "volume|create|--driver|local|data") {
		t.Fatalf("create = %+v, %v", created, err)
	}
	removed, err := volumeRemove("data")
	if err != nil || !strings.Contains(removed.Output, "volume|rm|data") {
		t.Fatalf("remove = %+v, %v", removed, err)
	}
	inspected, err := volumeInspect("data")
	if err != nil || inspected.Data != "volume|inspect|data" {
		t.Fatalf("inspect = %+v, %v", inspected, err)
	}
	for _, name := range []string{"", "../data", "a b", "x;rm -rf /"} {
		if _, err := volumeRemove(name); err == nil {
			t.Fatalf("volumeRemove(%q) must fail", name)
		}
	}
}

func TestContainerBatchAndInspectUseExactCommands(t *testing.T) {
	withDocker(t)
	result, err := containerBatch("restart", []string{"abc123", "def456"})
	if err != nil || !strings.Contains(result.Output, "container|restart|abc123|def456") {
		t.Fatalf("batch = %+v, %v", result, err)
	}
	inspected, err := containerInspect("abc123")
	if err != nil || inspected.Data != "container|inspect|abc123" {
		t.Fatalf("inspect = %+v, %v", inspected, err)
	}
}

func TestContainerBatchRejectsUnsafeInputs(t *testing.T) {
	for _, verb := range []string{"", "rm", "exec", "kill", "logs"} {
		if _, err := containerBatch(verb, []string{"abc123"}); err == nil {
			t.Fatalf("verb %q must fail", verb)
		}
	}
	if _, err := containerBatch("start", nil); err == nil {
		t.Fatal("empty id list must fail")
	}
	if _, err := containerBatch("start", []string{"abc123; rm -rf /"}); err == nil {
		t.Fatal("unsafe id must fail")
	}
	tooMany := make([]string, 101)
	for index := range tooMany {
		tooMany[index] = "abc"
	}
	if _, err := containerBatch("start", tooMany); err == nil {
		t.Fatal("oversized id list must fail")
	}
	if _, err := containerInspect("abc123; rm"); err == nil {
		t.Fatal("unsafe inspect id must fail")
	}
}

func TestComposeStatusLogsAndEnv(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Status")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := projectPath(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	withDocker(t, "ALX_DOCKER_HELPER_COMPOSE_PS=1")
	rows, err := composePS(project.ID)
	if err != nil || len(rows) != 1 || rows[0].Service != "web" || rows[0].State != "running" || rows[0].Name != "status-web-1" {
		t.Fatalf("ps = %+v, %v", rows, err)
	}
	logs, err := composeLogs(project.ID, 100)
	if err != nil || !strings.Contains(logs.Output, "compose|-f|"+composePath(dir)+"|logs|--tail|100|--no-color") {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
	written, err := composeEnvWrite(project.ID, "FOO=bar\n")
	if err != nil || strings.Contains(written.Output, "FOO") {
		t.Fatalf("env write must not echo content: %+v, %v", written, err)
	}
	read, err := composeEnvRead(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, ok := read.Data.(map[string]string)["content"]
	if !ok || content != "FOO=bar\n" {
		t.Fatalf("env content = %q", content)
	}
	if _, err := composeEnvWrite(project.ID, strings.Repeat("a", maxEnvBytes+1)); err == nil {
		t.Fatal("oversized env must fail")
	}
}

func TestComposeEnvWriteRejectsInvalidContent(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Env")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composeEnvWrite(project.ID, "A=B\x00C"); err == nil {
		t.Fatal("NUL bytes must fail")
	}
	if _, err := composeEnvWrite(project.ID, string([]byte{0xff, 0xfe})); err == nil {
		t.Fatal("invalid UTF-8 must fail")
	}
}

func TestExternalProjectsGroupsComposeLabels(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_CONTAINERS=1")
	projects, err := externalProjects()
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects = %+v, %v", projects, err)
	}
	entry := projects[0]
	if entry.Project != "myapp" || entry.ContainerCount != 1 || entry.RunningCount != 1 || entry.WorkingDir != "/srv/myapp" || entry.ConfigFiles != "/srv/myapp/compose.yml" || entry.Managed {
		t.Fatalf("external project = %+v", entry)
	}
}

func TestDeleteProjectRemovesOnlyManagedDirectory(t *testing.T) {
	root := useTemporaryProjectRoot(t)
	project, err := createProject("Doomed")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := projectPath(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := deleteProject(project.ID)
	if err != nil || !strings.Contains(result.Output, "已删除项目 Doomed") {
		t.Fatalf("delete = %+v, %v", result, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("project directory should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "..")); err != nil {
		t.Fatalf("parent directory must be untouched: %v", err)
	}
	if _, err := deleteProject("missing"); err == nil {
		t.Fatal("unknown project must fail")
	}
	if _, err := deleteProject("../escape"); err == nil {
		t.Fatal("unsafe project ID must fail")
	}
}

func withArgsFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("ALX_DOCKER_HELPER_ARGS_FILE", path)
	return path
}

func TestContainerCreateUsesExactCommandAndRedactsEnv(t *testing.T) {
	argsFile := withArgsFile(t)
	withDocker(t)
	result, err := containerCreate(map[string]string{
		"image":   "nginx:latest",
		"name":    "web",
		"restart": "unless-stopped",
		"network": "demo-net",
		"ports":   "8080:80\n127.0.0.1:9000:9000/udp",
		"env":     "FOO=secret-value\nDEBUG=1",
		"volumes": "data:/var/lib/data:ro",
		"labels":  "tier=web",
		"command": `echo "hello world"`,
	})
	if err != nil {
		t.Fatalf("create = %+v, %v", result, err)
	}
	got, readErr := os.ReadFile(argsFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	want := "run|-d|--name|web|--restart|unless-stopped|--network|demo-net|--publish|8080:80|--publish|127.0.0.1:9000:9000/udp|--env|FOO=secret-value|--env|DEBUG=1|--volume|data:/var/lib/data:ro|--label|tier=web|nginx:latest|echo|hello world"
	if string(got) != want {
		t.Fatalf("args = %q\nwant %q", got, want)
	}
	if strings.Contains(result.Output, "secret-value") || strings.Contains(result.Output, "--env") {
		t.Fatalf("env must never be echoed: %q", result.Output)
	}
}

func TestContainerCreateErrorRedactsEnv(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_NEXT=1")
	result, err := containerCreate(map[string]string{"image": "nginx:latest", "env": "FOO=topsecret"})
	if err == nil || !strings.Contains(err.Error(), "创建容器失败") {
		t.Fatalf("error should map docker failure: %v", err)
	}
	if strings.Contains(err.Error(), "topsecret") || strings.Contains(result.Output, "topsecret") {
		t.Fatalf("env must be redacted from errors/output: %q / %q", err, result.Output)
	}
}

func TestContainerCreateRejectsUnsafeInputs(t *testing.T) {
	cases := []map[string]string{
		{"image": "nginx; rm -rf /"},
		{"image": "nginx", "name": "../escape"},
		{"image": "nginx", "restart": "always --privileged"},
		{"image": "nginx", "ports": "-p 80:80"},
		{"image": "nginx", "ports": "8080; rm -rf /"},
		{"image": "nginx", "env": "-e FOO=1"},
		{"image": "nginx", "env": "BAD NAME=x"},
		{"image": "nginx", "volumes": "/data:/data"},
		{"image": "nginx", "volumes": "C:/data:/data"},
		{"image": "nginx", "volumes": "data:/data:exec"},
		{"image": "nginx", "labels": "-tier=web"},
		{"image": "nginx", "command": `echo "unclosed`},
		{"image": "nginx", "command": "a\x00b"},
	}
	for _, params := range cases {
		if _, err := containerCreate(params); err == nil {
			t.Fatalf("containerCreate(%v) must fail", params)
		}
	}
	tooMany := strings.Repeat("8080:80\n", 51)
	if _, err := containerCreate(map[string]string{"image": "nginx", "ports": tooMany}); err == nil {
		t.Fatal("too many ports must fail")
	}
}

func TestContainerCreateMinimalUsesDetachedRun(t *testing.T) {
	argsFile := withArgsFile(t)
	withDocker(t)
	result, err := containerCreate(map[string]string{"image": "busybox", "command": "sleep 3600"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(argsFile)
	if string(got) != "run|-d|busybox|sleep|3600" {
		t.Fatalf("args = %q", got)
	}
	if !strings.Contains(result.Output, "已创建容器") || result.Data == nil {
		t.Fatalf("result = %+v", result)
	}
}

func TestContainerStatsParsesRow(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_STATS=1")
	stat, err := containerStats("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if stat.ID != "abc123" || stat.Name != "web-1" || stat.CPUPerc != "0.05%" || stat.MemUsage != "5MiB / 8GiB" || stat.MemPerc != "0.10%" || stat.NetIO != "1.5kB / 2kB" || stat.BlockIO != "1.2kB / 0B" || stat.PIDs != "3" {
		t.Fatalf("stat = %+v", stat)
	}
}

func TestContainerLogsSinceUsesExactCommand(t *testing.T) {
	argsFile := withArgsFile(t)
	withDocker(t)
	if _, err := containerLogsSince("abc123", "2026-08-15T12:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(argsFile)
	if string(got) != "container|logs|--since|2026-08-15T12:00:00Z|--tail|100|--timestamps|--no-color|abc123" {
		t.Fatalf("args = %q", got)
	}
	if _, err := containerLogsSince("abc123", "yesterday", 100); err == nil || !strings.Contains(err.Error(), "时间戳无效") {
		t.Fatalf("invalid since must fail: %v", err)
	}
	if _, err := containerLogsSince("../x", "2026-08-15T12:00:00Z", 100); err == nil {
		t.Fatal("unsafe id must fail")
	}
}

func TestRegistryListReturnsKeysOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"auths":{"registry.example.com":{"auth":"c2VjcmV0"}},"credsStore":"desktop"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dir)
	entries, err := registryList()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Registry != "registry.example.com" || !entries[0].ExternalKey {
		t.Fatalf("entries = %+v", entries)
	}
	payload, _ := json.Marshal(entries)
	if strings.Contains(string(payload), "c2VjcmV0") {
		t.Fatalf("credentials leaked: %s", payload)
	}
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	empty, err := registryList()
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing config must be empty: %+v, %v", empty, err)
	}
}

func TestRegistryLoginUsesPasswordStdinAndRedacts(t *testing.T) {
	withDocker(t)
	var stdin string
	original := dockerInputSink
	dockerInputSink = func(input string) { stdin = input }
	t.Cleanup(func() { dockerInputSink = original })
	result, err := registryLogin("registry.example.com:5000", "user", "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if stdin != "hunter2\n" {
		t.Fatalf("stdin = %q", stdin)
	}
	if !strings.Contains(result.Output, "login|--username|user|--password-stdin|registry.example.com:5000") {
		t.Fatalf("login command = %q", result.Output)
	}
	if strings.Contains(result.Output, "hunter2") {
		t.Fatalf("password leaked: %q", result.Output)
	}
}

func TestRegistryLoginErrorRedactsPassword(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_NEXT=1")
	result, err := registryLogin("registry.example.com", "user", "hunter2")
	if err == nil || !strings.Contains(err.Error(), "登录失败") {
		t.Fatalf("error should map docker failure: %v", err)
	}
	if strings.Contains(err.Error(), "hunter2") || strings.Contains(result.Output, "hunter2") {
		t.Fatalf("password must be redacted: %q / %q", err, result.Output)
	}
}

func TestRegistryActionsValidateInputs(t *testing.T) {
	withDocker(t)
	for _, registry := range []string{"", "https://registry.example.com/path", "registry example.com", "-registry", "a/b"} {
		if _, err := registryLogout(registry); err == nil {
			t.Fatalf("registry %q must fail", registry)
		}
		if _, err := registryLogin(registry, "user", "pw"); err == nil {
			t.Fatalf("login registry %q must fail", registry)
		}
	}
	if _, err := registryLogin("registry.example.com", "bad user", "pw"); err == nil {
		t.Fatal("username with space must fail")
	}
	if _, err := registryLogin("registry.example.com", "user", strings.Repeat("a", 4097)); err == nil {
		t.Fatal("oversized password must fail")
	}
	if _, err := registryLogin("registry.example.com", "user", "pw\x00"); err == nil {
		t.Fatal("password with NUL must fail")
	}
}

func TestRegistryLogoutUsesExactCommand(t *testing.T) {
	argsFile := withArgsFile(t)
	withDocker(t)
	result, err := registryLogout("registry.example.com")
	if err != nil || !strings.Contains(result.Output, "已退出 registry.example.com") {
		t.Fatalf("logout = %+v, %v", result, err)
	}
	got, _ := os.ReadFile(argsFile)
	if string(got) != "logout|registry.example.com" {
		t.Fatalf("args = %q", got)
	}
}
