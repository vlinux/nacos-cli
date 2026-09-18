package nacos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	// configPageSize 列表接口单页大小
	configPageSize = 500
	// maxConfigPages 列表接口最多翻多少页，防止异常情况下死循环
	maxConfigPages = 200
)

// Get获取配置
//
// 注意：Nacos 对「配置不存在」并不总是返回 404，
// 实测 2.4.3 是 HTTP 200 + 空响应体，两种都要当成不存在处理。
func (c *Client) Get(operation ConfigGetOperation) (*NacosConfigDetail, error) {
	configURL, err := c.configURL()
	if err != nil {
		return nil, err
	}

	body, err := c.do(http.MethodGet, configURL, url.Values{
		"show":   {"all"},
		"dataId": {operation.DataId},
		"group":  {operation.Group},
		"tenant": {tenantOf(operation.Namespace)},
	}, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, notFoundError(operation)
		}
		return nil, err
	}

	if strings.TrimSpace(string(body)) == "" {
		return nil, notFoundError(operation)
	}

	detail := NacosConfigDetail{}
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("解析配置响应失败: %w。服务端返回: %s", err, compactBody(body))
	}
	return &detail, nil
}

func notFoundError(operation ConfigGetOperation) error {
	return fmt.Errorf("配置不存在: namespace=%s group=%s dataId=%s",
		displayNamespace(operation.Namespace), operation.Group, operation.DataId)
}

// displayNamespace 仅用于报错展示，把内部表示还原成用户看得懂的名字
func displayNamespace(namespace string) string {
	if tenantOf(namespace) == "" {
		return PublicNamespace
	}
	return namespace
}

// AllConfig 获取指定命名空间下的所有配置（自动翻页）
func (c *Client) AllConfig(operation ConfigGetOperation) ([]NacosPageItem, error) {
	configURL, err := c.configURL()
	if err != nil {
		return nil, err
	}

	items := make([]NacosPageItem, 0, configPageSize)

	for page := 1; page <= maxConfigPages; page++ {
		// dataId / group 传空串表示「不过滤」，但不能整个省略，否则服务端会报缺少参数
		body, err := c.do(http.MethodGet, configURL, url.Values{
			"dataId":   {operation.DataId},
			"group":    {operation.Group},
			"tenant":   {tenantOf(operation.Namespace)},
			"pageNo":   {strconv.Itoa(page)},
			"pageSize": {strconv.Itoa(configPageSize)},
			"search":   {"accurate"},
		}, nil)
		if err != nil {
			return nil, err
		}

		result := NacosPageResult{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析配置列表失败: %w。服务端返回: %s", err, compactBody(body))
		}

		items = append(items, result.PageItems...)
		if len(result.PageItems) < configPageSize {
			break
		}
	}

	return items, nil
}

// Edit 更新配置
func (c *Client) Edit(operation ConfigEditOperation) error {
	configURL, err := c.configURL()
	if err != nil {
		return err
	}

	body, err := c.do(http.MethodPost, configURL, nil, url.Values{
		"dataId":  {operation.DataId},
		"group":   {operation.Group},
		"content": {operation.Content},
		"tenant":  {tenantOf(operation.Namespace)},
		"type":    {operation.Type},
	})
	if err != nil {
		return err
	}

	// 发布成功时服务端返回 "true"
	if strings.TrimSpace(string(body)) == "false" {
		return fmt.Errorf("配置发布未生效：服务端返回 false (namespace=%s group=%s dataId=%s)",
			displayNamespace(operation.Namespace), operation.Group, operation.DataId)
	}
	return nil
}

// ApplyConfig 新增 or 修改配置
func (c *Client) ApplyConfig(operation ConfigApplyOperation) error {
	file, err := os.Open(operation.File)
	if err != nil {
		return err
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	operation.Type = resolveConfigType(operation.Type, operation.File)

	if operation.DataId == "" {
		operation.DataId = path.Base(operation.File)
	}

	if err = c.Edit(ConfigEditOperation{
		NacosOperation: operation.NacosOperation,
		Content:        string(buf),
		DataId:         operation.DataId,
		Type:           operation.Type,
	}); err != nil {
		return err
	}

	fmt.Printf("OK! 已发布 %s (namespace=%s group=%s)\n",
		operation.DataId, displayNamespace(operation.Namespace), operation.Group)
	return nil
}

// resolveConfigType 未显式指定类型时按文件后缀推断
func resolveConfigType(fileType, file string) string {
	if fileType != "" {
		return fileType
	}
	ext := strings.TrimPrefix(path.Ext(file), ".")
	if ext == "" {
		return ""
	}
	return ext
}

// DeleteConfig 删除配置
func (c *Client) DeleteConfig(operation ConfigDeleteOperation) error {
	configURL, err := c.configURL()
	if err != nil {
		return err
	}

	body, err := c.do(http.MethodDelete, configURL, url.Values{
		"dataId": {operation.DataId},
		"group":  {operation.Group},
		"tenant": {tenantOf(operation.Namespace)},
	}, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("配置不存在，无法删除: namespace=%s group=%s dataId=%s",
				displayNamespace(operation.Namespace), operation.Group, operation.DataId)
		}
		return err
	}

	if strings.TrimSpace(string(body)) == "false" {
		return fmt.Errorf("配置删除未生效：服务端返回 false (namespace=%s group=%s dataId=%s)",
			displayNamespace(operation.Namespace), operation.Group, operation.DataId)
	}
	return nil
}
