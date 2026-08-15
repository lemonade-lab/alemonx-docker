package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxComposeBytes = 2 << 20

var userConfigDir = os.UserConfigDir
var projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

type ProjectMeta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Project struct {
	ProjectMeta
	Content string `json:"content,omitempty"`
}

type ProjectImportTarget struct {
	Project     ProjectMeta `json:"project"`
	Destination string      `json:"destination"`
}

func projectRoot() (string, error) {
	config, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户配置目录：%w", err)
	}
	return filepath.Join(config, "alx-docker", "projects"), nil
}

func projectPath(id string) (string, error) {
	if !projectIDPattern.MatchString(strings.TrimSpace(id)) {
		return "", errors.New("项目 ID 无效")
	}
	root, err := projectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, id), nil
}

func projectID(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 || strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		return "", errors.New("项目名称需为 1–64 个字符，且不能包含路径分隔符")
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "project"
	}
	if base[0] >= '0' && base[0] <= '9' {
		base = "project-" + base
	}
	hash := sha256.Sum256([]byte(name))
	if len(base) > 48 {
		base = strings.TrimRight(base[:48], "-")
	}
	return base + "-" + hex.EncodeToString(hash[:])[:8], nil
}

func metaPath(dir string) string    { return filepath.Join(dir, "project.json") }
func composePath(dir string) string { return filepath.Join(dir, "docker-compose.yml") }

func listProjects() ([]ProjectMeta, error) {
	root, err := projectRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []ProjectMeta{}, nil
	}
	if err != nil {
		return nil, err
	}
	projects := make([]ProjectMeta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !projectIDPattern.MatchString(entry.Name()) {
			continue
		}
		data, readErr := os.ReadFile(metaPath(filepath.Join(root, entry.Name())))
		if readErr != nil {
			continue
		}
		var meta ProjectMeta
		if json.Unmarshal(data, &meta) == nil && meta.ID == entry.Name() {
			projects = append(projects, meta)
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].UpdatedAt > projects[j].UpdatedAt })
	return projects, nil
}

func readProject(id string) (Project, error) {
	dir, err := projectPath(id)
	if err != nil {
		return Project{}, err
	}
	meta, err := readMeta(dir)
	if err != nil {
		return Project{}, err
	}
	content, err := os.ReadFile(composePath(dir))
	if err != nil {
		return Project{}, fmt.Errorf("无法读取 docker-compose.yml：%w", err)
	}
	return Project{ProjectMeta: meta, Content: string(content)}, nil
}

func readMeta(dir string) (ProjectMeta, error) {
	data, err := os.ReadFile(metaPath(dir))
	if err != nil {
		return ProjectMeta{}, fmt.Errorf("找不到项目：%w", err)
	}
	var meta ProjectMeta
	if err := json.Unmarshal(data, &meta); err != nil || !projectIDPattern.MatchString(meta.ID) || meta.Name == "" {
		return ProjectMeta{}, errors.New("项目元数据无效")
	}
	return meta, nil
}

func createProject(name string) (Project, error) {
	return createProjectWithContent(name, "新建", "services: {}\n")
}

func projectImportTarget(id string) (ProjectImportTarget, error) {
	dir, err := projectPath(id)
	if err != nil {
		return ProjectImportTarget{}, err
	}
	meta, err := readMeta(dir)
	if err != nil {
		return ProjectImportTarget{}, err
	}
	return ProjectImportTarget{Project: meta, Destination: dir}, nil
}

func createProjectWithContent(name, source, content string) (Project, error) {
	id, err := projectID(name)
	if err != nil {
		return Project{}, err
	}
	if err := validateCompose(content); err != nil {
		return Project{}, err
	}
	dir, err := projectPath(id)
	if err != nil {
		return Project{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return Project{}, err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		if os.IsExist(err) {
			return Project{}, errors.New("同名项目已存在")
		}
		return Project{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	meta := ProjectMeta{ID: id, Name: strings.TrimSpace(name), Source: source, CreatedAt: now, UpdatedAt: now}
	if err := writeFileAtomic(composePath(dir), []byte(content), 0600); err != nil {
		_ = os.RemoveAll(dir)
		return Project{}, err
	}
	if err := writeMeta(dir, meta); err != nil {
		_ = os.RemoveAll(dir)
		return Project{}, err
	}
	return Project{ProjectMeta: meta, Content: content}, nil
}

func saveProject(id, content string) (Project, error) {
	if len(content) > maxComposeBytes {
		return Project{}, errors.New("Compose 文件不能超过 2 MiB")
	}
	if err := validateCompose(content); err != nil {
		return Project{}, err
	}
	dir, err := projectPath(id)
	if err != nil {
		return Project{}, err
	}
	meta, err := readMeta(dir)
	if err != nil {
		return Project{}, err
	}
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeFileAtomic(composePath(dir), []byte(content), 0600); err != nil {
		return Project{}, err
	}
	if err := writeMeta(dir, meta); err != nil {
		return Project{}, err
	}
	return Project{ProjectMeta: meta, Content: content}, nil
}

func writeMeta(dir string, meta ProjectMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(metaPath(dir), data, 0600)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".alx-compose-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func importUploadedProject(name, stagingDir, destination string) (Project, error) {
	if strings.TrimSpace(stagingDir) == "" || !filepath.IsAbs(stagingDir) {
		return Project{}, errors.New("上传暂存目录无效")
	}
	if strings.TrimSpace(name) != "" {
		return Project{}, errors.New("上传请求不应直接传入项目名称")
	}
	root, err := projectRoot()
	if err != nil {
		return Project{}, err
	}
	destination = filepath.Clean(destination)
	relative, err := filepath.Rel(root, destination)
	if err != nil || relative == "." || strings.Contains(relative, string(filepath.Separator)) || !projectIDPattern.MatchString(relative) {
		return Project{}, errors.New("上传目标无效")
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return Project{}, errors.New("无法读取上传文件")
	}
	if len(entries) != 1 || entries[0].IsDir() || !isComposeFilename(entries[0].Name()) {
		return Project{}, errors.New("请拖入一个 docker-compose.yml、docker-compose.yaml、compose.yml 或 compose.yaml 文件")
	}
	data, err := readLimitedFile(filepath.Join(stagingDir, entries[0].Name()))
	if err != nil {
		return Project{}, err
	}
	project, err := saveProject(relative, string(data))
	if err != nil {
		return Project{}, err
	}
	dir, _ := projectPath(relative)
	project.Source = "本地导入"
	if err := writeMeta(dir, project.ProjectMeta); err != nil {
		return Project{}, err
	}
	return project, nil
}

func downloadProject(name, rawURL string) (Project, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return Project{}, errors.New("下载地址必须是有效的 HTTPS URL")
	}
	if err := validatePublicDownloadHost(u.Hostname()); err != nil {
		return Project{}, err
	}
	data, err := downloadCompose(u.String())
	if err != nil {
		return Project{}, err
	}
	return createProjectWithContent(name, "下载："+u.String(), string(data))
}

func validatePublicDownloadHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("下载地址不能指向本机或内网")
	}
	if parsed := net.ParseIP(host); parsed != nil {
		if !isPublicIP(parsed) {
			return errors.New("下载地址不能指向本机或内网")
		}
		return nil
	}
	addresses, err := net.LookupIP(host)
	if err != nil || len(addresses) == 0 {
		return errors.New("下载地址无法解析为公网地址")
	}
	for _, address := range addresses {
		if !isPublicIP(address) {
			return errors.New("下载地址不能指向本机或内网")
		}
	}
	return nil
}

func isPublicIP(address net.IP) bool {
	return !(address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() || address.IsMulticast())
}

func downloadCompose(rawURL string) ([]byte, error) {
	broker, token := os.Getenv("ALX_PLUGIN_DOWNLOAD_BROKER"), os.Getenv("ALX_PLUGIN_DOWNLOAD_TOKEN")
	if broker == "" || token == "" {
		return nil, errors.New("宿主下载代理不可用，无法安全下载 Compose 文件")
	}
	request, err := http.NewRequest(http.MethodGet, broker+"?url="+url.QueryEscape(rawURL), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := (&http.Client{Timeout: 45 * time.Second}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("下载失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("下载失败（HTTP %d）", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxComposeBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxComposeBytes {
		return nil, errors.New("Compose 文件不能超过 2 MiB")
	}
	return data, nil
}

func readLimitedFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > maxComposeBytes {
		return nil, errors.New("Compose 文件不能超过 2 MiB")
	}
	return data, nil
}

func isComposeFilename(name string) bool {
	switch strings.ToLower(name) {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	default:
		return false
	}
}
