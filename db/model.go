package db

import "time"

const (
	InDir, OutDir = "i", "o"
)

type SystemInfo struct {
	ID                uint `gorm:"primaryKey"`
	SystemName        string
	SystemDescription string
	TrafficRecordDays uint
}

// 从属关系，所有者删除时被所有者应为0或者同时删除，被所有者删除时清除关联(多对多时)
// 关联关系，删除时清除关联关系
type User struct {
	ID          string    `gorm:"primaryKey"`
	Password    string    `gorm:"not null"`
	LinkToken   string    //可选，用于代理url token
	Email       string    `gorm:"uniqueIndex;not null"`
	Enabled     bool      `gorm:"default:true"`
	UserGroupID string    `gorm:"not null"`
	UserGroup   UserGroup `gorm:"foreignKey:UserGroupID"` // 所属用户组
}

type UserGroup struct {
	ID            string      `gorm:"primaryKey"`
	RouteSchemeID string      `gorm:"not null"`
	RouteScheme   RouteScheme `gorm:"foreignKey:RouteSchemeID"`             //所属的RouteScheme
	Users         []User      `gorm:"foreignKey:UserGroupID"`               // 拥有的用户
	AvailInbounds []ProxyData `gorm:"many2many:user_group_avail_inbounds;"` //关联的 ProxyData
}

type ProxyData struct {
	ID        string `gorm:"primaryKey"`
	Direction string `gorm:"not null"`

	Enabled    bool        `gorm:"default:true"`
	Config     string      `gorm:"type:json"`                            // JSON 存储
	UserGroups []UserGroup `gorm:"many2many:user_group_avail_inbounds;"` //关联用户组
}

type RouteScheme struct {
	ID          string `gorm:"primaryKey"`
	Description string
	Enabled     bool        `gorm:"default:true"`
	Rules       []Rule      `gorm:"foreignKey:RouteSchemeID"` // 拥有的规则
	UserGroups  []UserGroup `gorm:"foreignKey:RouteSchemeID"` // 拥有的用户组
}
type Rule struct {
	ID            uint `gorm:"primaryKey"`
	Name          string
	Type          string
	Pattern       string
	RouteSchemeID string      `gorm:"not null"`
	RouteScheme   RouteScheme `gorm:"foreignKey:RouteSchemeID"`  // 所属的 RouteScheme
	Outbounds     []ProxyData `gorm:"many2many:rule_outbounds;"` // 关联的 ProxyData
	Priority      uint        `gorm:"default:0"`                 // 优先级，值越小优先级越高
}

type Traffic struct {
	ID         uint      `gorm:"primaryKey"`
	InboundID  string    `gorm:"not null"`
	Inbound    ProxyData `gorm:"foreignKey:InboundID"`
	OutboundID string    `gorm:"not null"`
	Outbound   ProxyData `gorm:"foreignKey:OutboundID"`
	UserID     string    `gorm:"not null"`
	User       User      `gorm:"foreignKey:UserID"` // 关联的用户
	BytesIn    uint64    `gorm:"default:0"`         // 入站流量
	BytesOut   uint64    `gorm:"default:0"`         // 出站流量
	DestAddr   string    `gorm:"not null"`
	Time       time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}
