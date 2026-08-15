package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var registryHostReference = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?(:[0-9]{1,5})?$`)
var registryUsernameReference = regexp.MustCompile(`^[^\s/]{1,128}$`)

type RegistryEntry struct {
	Registry    string `json:"registry"`
	ExternalKey bool   `json:"externalKey"`
}

// dockerConfigPath locates ~/.docker/config.json, honouring DOCKER_CONFIG.
func dockerConfigPath() string {
	if dir := strings.TrimSpace(os.Getenv("DOCKER_CONFIG")); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker", "config.json")
}

// registryList returns only the registry keys configured in Docker's auths
// map. Credentials are never decoded or echoed; a credsStore/credHelpers
// presence is surfaced as an external-key flag instead.
func registryList() ([]RegistryEntry, error) {
	path := dockerConfigPath()
	if path == "" {
		return []RegistryEntry{}, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []RegistryEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 Docker 配置失败：%w", err)
	}
	var raw struct {
		Auths       map[string]json.RawMessage `json:"auths"`
		CredsStore  string                     `json:"credsStore"`
		CredHelpers map[string]string          `json:"credHelpers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("Docker 配置文件格式无效")
	}
	external := raw.CredsStore != "" || len(raw.CredHelpers) > 0
	entries := []RegistryEntry{}
	for registry := range raw.Auths {
		entries = append(entries, RegistryEntry{Registry: registry, ExternalKey: external})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Registry < entries[j].Registry })
	return entries, nil
}

func validateRegistryHost(registry string) error {
	if len(registry) > 253 || !registryHostReference.MatchString(registry) {
		return errors.New("仓库地址无效（仅支持 域名[:端口]，不能包含路径）")
	}
	return nil
}

// registryLogin runs `docker login --username <u> --password-stdin <registry>`.
// The password travels only on stdin, never in argv, output or errors.
func registryLogin(registry, username, password string) (actionResult, error) {
	registry = strings.TrimSpace(registry)
	username = strings.TrimSpace(username)
	if err := validateRegistryHost(registry); err != nil {
		return actionResult{}, err
	}
	if !registryUsernameReference.MatchString(username) {
		return actionResult{}, errors.New("用户名无效")
	}
	if len(password) > 4096 || strings.ContainsRune(password, '\x00') {
		return actionResult{}, errors.New("密码无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDockerInput("", password+"\n", "login", "--username", username, "--password-stdin", registry)
	if err != nil {
		text := redactSecrets(shortDockerError(err, output), []string{password})
		return actionResult{Output: redactSecrets(output, []string{password})}, fmt.Errorf("登录失败：%s", text)
	}
	return actionResult{Output: "✓ 已登录 " + registry + "（用户 " + username + "，凭据不会回显）。\n" + clipOutput(output)}, nil
}

func registryLogout(registry string) (actionResult, error) {
	registry = strings.TrimSpace(registry)
	if err := validateRegistryHost(registry); err != nil {
		return actionResult{}, err
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "logout", registry)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("退出登录失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已退出 " + registry + "。\n" + clipOutput(output)}, nil
}
