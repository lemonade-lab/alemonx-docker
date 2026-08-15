package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const maxEnvBytes = 64 << 10

type ComposeServiceStatus struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Service string `json:"service"`
	State   string `json:"state"`
	Status  string `json:"status,omitempty"`
	Image   string `json:"image,omitempty"`
}

// composeInProject runs a whitelisted docker compose subcommand with the fixed
// managed compose file and the project directory as the working directory.
func composeInProject(id string, args ...string) (string, error) {
	if err := requireDocker(); err != nil {
		return "", err
	}
	dir, err := projectPath(id)
	if err != nil {
		return "", err
	}
	if _, err := readMeta(dir); err != nil {
		return "", err
	}
	full := append([]string{"compose", "-f", composePath(dir)}, args...)
	return runDocker(dir, full...)
}

func composePS(id string) ([]ComposeServiceStatus, error) {
	output, err := composeInProject(id, "ps", "-a", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("读取项目状态失败：%s", shortDockerError(err, output))
	}
	items := []ComposeServiceStatus{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw struct {
			ID      string `json:"ID"`
			Name    string `json:"Name"`
			Service string `json:"Service"`
			State   string `json:"State"`
			Status  string `json:"Status"`
			Image   string `json:"Image"`
		}
		if json.Unmarshal([]byte(line), &raw) == nil && raw.Name != "" {
			items = append(items, ComposeServiceStatus{raw.ID, raw.Name, raw.Service, raw.State, raw.Status, raw.Image})
		}
	}
	return items, nil
}

func composeLogs(id string, lines int) (actionResult, error) {
	if lines < 1 {
		lines = 1
	}
	if lines > 5000 {
		lines = 5000
	}
	output, err := composeInProject(id, "logs", "--tail", strconv.Itoa(lines), "--no-color")
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("读取项目日志失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: clipOutput(output)}, nil
}

func composeEnvRead(id string) (actionResult, error) {
	dir, err := projectPath(id)
	if err != nil {
		return actionResult{}, err
	}
	if _, err := readMeta(dir); err != nil {
		return actionResult{}, err
	}
	content, err := os.ReadFile(filepath.Join(dir, ".env"))
	if os.IsNotExist(err) {
		return actionResult{Output: "没有找到 .env 文件（将按空文件处理）。", Data: map[string]string{"content": ""}}, nil
	}
	if err != nil {
		return actionResult{}, fmt.Errorf("读取 .env 失败：%w", err)
	}
	if len(content) > maxEnvBytes {
		return actionResult{}, errors.New(".env 文件不能超过 64 KiB")
	}
	return actionResult{Output: "✓ 已读取 .env（内容仅用于编辑，不回显到任务记录）。", Data: map[string]string{"content": string(content)}}, nil
}

func composeEnvWrite(id, content string) (actionResult, error) {
	if len(content) > maxEnvBytes {
		return actionResult{}, errors.New(".env 文件不能超过 64 KiB")
	}
	if strings.ContainsRune(content, '\x00') || !utf8.ValidString(content) {
		return actionResult{}, errors.New(".env 内容无效")
	}
	dir, err := projectPath(id)
	if err != nil {
		return actionResult{}, err
	}
	meta, err := readMeta(dir)
	if err != nil {
		return actionResult{}, err
	}
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeFileAtomic(filepath.Join(dir, ".env"), []byte(content), 0600); err != nil {
		return actionResult{}, err
	}
	if err := writeMeta(dir, meta); err != nil {
		return actionResult{}, err
	}
	return actionResult{Output: "✓ 已保存 .env（内容不会回显到任务记录）。", Data: meta}, nil
}

func validateCompose(content string) error {
	if strings.TrimSpace(content) == "" {
		return errors.New("docker-compose.yml 不能为空")
	}
	if len(content) > maxComposeBytes {
		return errors.New("Compose 文件不能超过 2 MiB")
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return fmt.Errorf("Compose YAML 无效：%w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return errors.New("Compose 根节点必须是对象")
	}
	for i := 0; i+1 < len(root.Content[0].Content); i += 2 {
		if root.Content[0].Content[i].Value == "services" && root.Content[0].Content[i+1].Kind != yaml.MappingNode {
			return errors.New("Compose 的 services 必须是对象")
		}
	}
	return nil
}
