package nacos

// PublicNamespace Nacos 控制台里显示的公共命名空间名称。
// 注意：它的真实命名空间 ID 是空字符串，API 的 tenant 参数必须传空串。
const PublicNamespace = "public"

type NacosConfig struct {
	Addr        string `json:"addr" yaml:"addr"`
	Username    string `json:"username" yaml:"username"`
	Password    string `json:"password" yaml:"password"`
	ApiVersion  string `json:"apiVersion" yaml:"apiVersion"`
	Namespace   string `json:"namespace" yaml:"namespace"`
	Group       string `json:"group" yaml:"group"`
	AccessToken string `json:"accessToken" yaml:"accessToken"`
	// Insecure 跳过 HTTPS 证书校验，用于内网自签证书的 Nacos
	Insecure bool `json:"insecure" yaml:"insecure"`
}

type NacosOperation struct {
	Namespace string // 命名空间
	Group     string // 组
}

// ConfigEditOperation 配置更新操作
type ConfigEditOperation struct {
	*NacosOperation
	Content string // 配置内容
	DataId  string // data-id
	Type    string // 文件类型
}

// ConfigEditOperation 配置查询操作
type ConfigGetOperation struct {
	*NacosOperation
	DataId string // data-id
}

// ConfigApplyOperation 配置更新操作
type ConfigApplyOperation struct {
	*NacosOperation
	File   string // 配置文件
	DataId string // data-id
	Type   string // 文件类型
}

// ConfigDeleteOperation 配置删除操作
type ConfigDeleteOperation struct {
	*NacosOperation
	DataId string // data-id
}

var DefaultNacosOperation = NacosOperation{
	Namespace: PublicNamespace,
	Group:     "DEFAULT_GROUP",
}

type NacosPageResult struct {
	TotalCount     int             `json:"totalCount"`
	PageNumber     int             `json:"pageNumber"`
	PagesAvailable int             `json:"pagesAvailable"`
	PageItems      []NacosPageItem `json:"pageItems"`
}

type NacosPageItem struct {
	Id     string `json:"id"`
	DataId string `json:"dataId"`
	Group  string `json:"group"`
	Type   string `json:"type"`   // 文件类型
	Tenant string `json:"tenant"` // 命名空间
}

// NacosConfigDetail nacos配置结构体
type NacosConfigDetail struct {
	ID               string `json:"id"`
	DataID           string `json:"dataId"`
	Group            string `json:"group"`
	Content          string `json:"content"`
	Md5              string `json:"md5"`
	EncryptedDataKey string `json:"encryptedDataKey"`
	Tenant           string `json:"tenant"`
	AppName          string `json:"appName"`
	Type             string `json:"type"`
	CreateTime       int64  `json:"createTime"`
	ModifyTime       int64  `json:"modifyTime"`
	CreateUser       string `json:"createUser"`
	CreateIP         string `json:"createIp"`
	Desc             string `json:"desc"`
	Use              string `json:"use"`
	Effect           string `json:"effect"`
	Schema           string `json:"schema"`
}

// LoginResult Nacos 登录接口返回体。
// v1 的 /auth/login 与 v3 的 /auth/user/login 字段兼容，v3 会多返回 username。
type LoginResult struct {
	AccessToken string `json:"accessToken"`
	TokenTTL    int64  `json:"tokenTtl"`
	GlobalAdmin bool   `json:"globalAdmin"`
	Username    string `json:"username"`
}
