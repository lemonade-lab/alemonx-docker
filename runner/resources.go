package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

type DockerNetwork struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type DockerVolume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Scope      string `json:"scope"`
	Mountpoint string `json:"mountpoint,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
}

type ExternalProject struct {
	Project        string `json:"project"`
	WorkingDir     string `json:"workingDir,omitempty"`
	ConfigFiles    string `json:"configFiles,omitempty"`
	ContainerCount int    `json:"containerCount"`
	RunningCount   int    `json:"runningCount"`
	Managed        bool   `json:"managed"`
}

func dockerNetworks() ([]DockerNetwork, error) {
	if err := requireDocker(); err != nil {
		return nil, err
	}
	output, err := runDocker("", "network", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("读取网络失败：%s", shortDockerError(err, output))
	}
	items := []DockerNetwork{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw struct {
			ID        string
			Name      string
			Driver    string
			Scope     string
			CreatedAt string
		}
		if json.Unmarshal([]byte(line), &raw) == nil && raw.Name != "" {
			items = append(items, DockerNetwork{raw.ID, raw.Name, raw.Driver, raw.Scope, raw.CreatedAt})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func networkInspect(name string) (actionResult, error) {
	name = strings.TrimSpace(name)
	if !networkReference.MatchString(name) {
		return actionResult{}, errors.New("网络名称无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "network", "inspect", name)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("读取网络详情失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已读取网络 " + name + " 详情", Data: parseJSONOutput(output)}, nil
}

func networkCreate(name, driver, subnet, gateway string) (actionResult, error) {
	name = strings.TrimSpace(name)
	if !networkReference.MatchString(name) {
		return actionResult{}, errors.New("网络名称无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	args := []string{"network", "create"}
	if driver = strings.TrimSpace(driver); driver != "" {
		if !wordReference.MatchString(driver) {
			return actionResult{}, errors.New("网络驱动无效")
		}
		args = append(args, "--driver", driver)
	}
	if subnet = strings.TrimSpace(subnet); subnet != "" {
		if _, _, err := net.ParseCIDR(subnet); err != nil {
			return actionResult{}, errors.New("子网必须是有效的 CIDR，例如 172.20.0.0/16")
		}
		args = append(args, "--subnet", subnet)
	}
	if gateway = strings.TrimSpace(gateway); gateway != "" {
		if net.ParseIP(gateway) == nil {
			return actionResult{}, errors.New("网关必须是有效的 IP 地址")
		}
		args = append(args, "--gateway", gateway)
	}
	args = append(args, name)
	output, err := runDocker("", args...)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("创建网络失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已创建网络 " + name + "。\n" + clipOutput(output)}, nil
}

func networkRemove(name string) (actionResult, error) {
	name = strings.TrimSpace(name)
	if !networkReference.MatchString(name) {
		return actionResult{}, errors.New("网络名称无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "network", "rm", name)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("删除网络失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已删除网络 " + name + "。\n" + clipOutput(output)}, nil
}

func dockerVolumes() ([]DockerVolume, error) {
	if err := requireDocker(); err != nil {
		return nil, err
	}
	output, err := runDocker("", "volume", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("读取卷失败：%s", shortDockerError(err, output))
	}
	items := []DockerVolume{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw struct {
			Name       string
			Driver     string
			Scope      string
			Mountpoint string
			CreatedAt  string
		}
		if json.Unmarshal([]byte(line), &raw) == nil && raw.Name != "" {
			items = append(items, DockerVolume{raw.Name, raw.Driver, raw.Scope, raw.Mountpoint, raw.CreatedAt})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func volumeInspect(name string) (actionResult, error) {
	name = strings.TrimSpace(name)
	if !volumeReference.MatchString(name) {
		return actionResult{}, errors.New("卷名称无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "volume", "inspect", name)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("读取卷详情失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已读取卷 " + name + " 详情", Data: parseJSONOutput(output)}, nil
}

func volumeCreate(name, driver string) (actionResult, error) {
	name = strings.TrimSpace(name)
	if !volumeReference.MatchString(name) {
		return actionResult{}, errors.New("卷名称无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	args := []string{"volume", "create"}
	if driver = strings.TrimSpace(driver); driver != "" {
		if !wordReference.MatchString(driver) {
			return actionResult{}, errors.New("卷驱动无效")
		}
		args = append(args, "--driver", driver)
	}
	args = append(args, name)
	output, err := runDocker("", args...)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("创建卷失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已创建卷 " + name + "。\n" + clipOutput(output)}, nil
}

func volumeRemove(name string) (actionResult, error) {
	name = strings.TrimSpace(name)
	if !volumeReference.MatchString(name) {
		return actionResult{}, errors.New("卷名称无效")
	}
	if err := requireDocker(); err != nil {
		return actionResult{}, err
	}
	output, err := runDocker("", "volume", "rm", name)
	if err != nil {
		return actionResult{Output: output}, fmt.Errorf("删除卷失败：%s", shortDockerError(err, output))
	}
	return actionResult{Output: "✓ 已删除卷 " + name + "。\n" + clipOutput(output)}, nil
}

// externalProjects discovers Compose projects running on this host through the
// com.docker.compose.project labels, complementing the managed project library.
func externalProjects() ([]ExternalProject, error) {
	items, err := dockerContainers()
	if err != nil {
		return nil, err
	}
	managed, err := listProjects()
	if err != nil {
		return nil, err
	}
	managedIDs := map[string]bool{}
	for _, project := range managed {
		managedIDs[project.ID] = true
	}
	grouped := map[string]*ExternalProject{}
	for _, item := range items {
		labels := containerLabels(item.Labels)
		name := labels["com.docker.compose.project"]
		if name == "" {
			continue
		}
		entry := grouped[name]
		if entry == nil {
			entry = &ExternalProject{
				Project:     name,
				WorkingDir:  labels["com.docker.compose.project.working_dir"],
				ConfigFiles: labels["com.docker.compose.project.config_files"],
				Managed:     managedIDs[name],
			}
			grouped[name] = entry
		}
		entry.ContainerCount++
		if item.State == "running" {
			entry.RunningCount++
		}
	}
	projects := make([]ExternalProject, 0, len(grouped))
	for _, entry := range grouped {
		projects = append(projects, *entry)
	}
	sort.SliceStable(projects, func(i, j int) bool { return projects[i].Project < projects[j].Project })
	return projects, nil
}
