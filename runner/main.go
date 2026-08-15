package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const protocol = "alx/v1"

type request struct {
	Protocol string            `json:"protocol"`
	Method   string            `json:"method"`
	Action   string            `json:"action"`
	Params   map[string]string `json:"params"`
}

type response struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
	Data   any    `json:"data,omitempty"`
}

type actionResult struct {
	Output string
	Data   any
}

func main() {
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		write(response{Error: "请求格式无效：" + err.Error()})
		return
	}
	if input.Protocol != protocol || input.Method != "run" {
		write(response{Error: fmt.Sprintf("不支持的 ALX 插件协议（protocol=%q method=%q）", input.Protocol, input.Method)})
		return
	}
	result, err := runAction(input.Action, input.Params)
	write(response{Output: result.Output, Data: result.Data, Error: errorText(err)})
}

func write(result response) { _ = json.NewEncoder(os.Stdout).Encode(result) }

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func statusResult(data any) (actionResult, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return actionResult{}, err
	}
	return actionResult{Output: string(payload), Data: data}, nil
}

func runAction(action string, params map[string]string) (actionResult, error) {
	switch action {
	case "docker-status":
		return statusResult(dockerStatus())
	case "project-list":
		projects, err := listProjects()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(projects)
	case "project-read":
		project, err := readProject(params["projectID"])
		if err != nil {
			return actionResult{}, err
		}
		return actionResult{Output: "✓ 已读取项目 " + project.Name, Data: project}, nil
	case "project-create":
		project, err := createProject(params["name"])
		if err != nil {
			return actionResult{}, err
		}
		return actionResult{Output: "✓ 已创建项目 " + project.Name, Data: project}, nil
	case "project-import-target":
		target, err := projectImportTarget(params["projectID"])
		if err != nil {
			return actionResult{}, err
		}
		return actionResult{Output: "✓ 已准备导入目录", Data: target}, nil
	case "project-save":
		project, err := saveProject(params["projectID"], params["content"])
		if err != nil {
			return actionResult{}, err
		}
		return actionResult{Output: "✓ 已保存 docker-compose.yml", Data: project}, nil
	case "project-download":
		project, err := downloadProject(params["name"], params["url"])
		if err != nil {
			return actionResult{}, err
		}
		return actionResult{Output: "✓ 已下载并保存项目 " + project.Name, Data: project}, nil
	case "project-import-example":
		project, err := importExampleProject(params["name"], params["example"])
		if err != nil {
			return actionResult{}, err
		}
		return actionResult{Output: "✓ 已导入示例项目 " + project.Name, Data: project}, nil
	case "recommendations":
		items, err := loadRecommendations()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	case "upload-compose":
		project, err := importUploadedProject(params["name"], params["stagingDir"], params["destination"])
		if err != nil {
			return actionResult{}, err
		}
		return actionResult{Output: "✓ 已导入项目 " + project.Name, Data: project}, nil
	case "compose-up", "compose-stop", "compose-restart", "compose-down":
		return composeAction(action, params["projectID"])
	case "image-list":
		items, err := dockerImages()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	case "image-pull":
		return imagePull(params["image"])
	case "image-remove":
		return imageRemove(params["image"])
	case "image-tag":
		return imageTag(params["image"], params["target"])
	case "image-push":
		return imagePush(params["image"])
	case "image-prune":
		return imagePrune(params["all"] == "1")
	case "container-list":
		items, err := dockerContainers()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	case "container-inspect":
		return containerInspect(params["containerID"])
	case "container-create":
		return containerCreate(params)
	case "container-stats":
		stat, err := containerStats(params["containerID"])
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(stat)
	case "container-batch":
		return containerBatch(params["verb"], splitReferences(params["containerIDs"]))
	case "container-logs-since":
		return containerLogsSince(params["containerID"], params["since"], intParam(params["lines"], 200))
	case "registry-list":
		entries, err := registryList()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(entries)
	case "registry-login":
		return registryLogin(params["registry"], params["username"], params["password"])
	case "registry-logout":
		return registryLogout(params["registry"])
	case "docker-ports":
		items, err := dockerPorts()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	case "container-logs":
		return containerLogs(params["containerID"])
	case "container-start", "container-stop", "container-restart":
		return containerAction(action, params["containerID"])
	case "network-list":
		items, err := dockerNetworks()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	case "network-inspect":
		return networkInspect(params["name"])
	case "network-create":
		return networkCreate(params["name"], params["driver"], params["subnet"], params["gateway"])
	case "network-remove":
		return networkRemove(params["name"])
	case "volume-list":
		items, err := dockerVolumes()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	case "volume-inspect":
		return volumeInspect(params["name"])
	case "volume-create":
		return volumeCreate(params["name"], params["driver"])
	case "volume-remove":
		return volumeRemove(params["name"])
	case "compose-ps":
		items, err := composePS(params["projectID"])
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	case "compose-logs":
		return composeLogs(params["projectID"], intParam(params["lines"], 200))
	case "compose-env-read":
		return composeEnvRead(params["projectID"])
	case "compose-env-write":
		return composeEnvWrite(params["projectID"], params["content"])
	case "project-delete":
		return deleteProject(params["projectID"])
	case "external-projects":
		items, err := externalProjects()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
	default:
		return actionResult{}, fmt.Errorf("未知操作：%s", action)
	}
}

func splitReferences(raw string) []string {
	items := []string{}
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			items = append(items, part)
		}
	}
	return items
}

func intParam(raw string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return value
	}
	return fallback
}
