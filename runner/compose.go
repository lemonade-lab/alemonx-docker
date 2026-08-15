package main

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
