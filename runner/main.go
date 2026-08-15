package main

import (
	"encoding/json"
	"fmt"
	"os"
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
	case "container-list":
		items, err := dockerContainers()
		if err != nil {
			return actionResult{}, err
		}
		return statusResult(items)
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
	default:
		return actionResult{}, fmt.Errorf("未知操作：%s", action)
	}
}
