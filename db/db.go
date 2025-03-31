package db

import (
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	MAX = 10000000
)

type RepoManager struct {
	DB          *gorm.DB
	User        *UserRepo
	UserGroup   *UserGroupRepo
	ProxyData   *ProxyDataRepo
	RouteScheme *RouteSchemeRepo
	Rule        *RuleRepo
	SystemInfo  *SystemInfoRepo
}
type StatisticRepoManager struct {
	DB      *gorm.DB
	Traffic *TrafficRepo
}

func OpenDB(dbPath string) (*gorm.DB, bool, error) {
	dbDir := filepath.Dir(dbPath)
	if dbDir != "." && dbDir != "" {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, false, err
		}
	}

	// 检查文件是否存在，不存在则创建
	var isNewDB bool
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		file, err := os.Create(dbPath)

		if err != nil {
			return nil, false, err
		}
		file.Close()
		isNewDB = true
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	return db, isNewDB, err
}
func InitRepo(dbPath string) (*RepoManager, bool, error) {

	db, isNewDB, err := OpenDB(dbPath)
	if err != nil {
		return nil, isNewDB, err
	}

	// 迁移数据库表结构
	err = db.AutoMigrate(
		&User{},
		&UserGroup{},
		&ProxyData{},
		&RouteScheme{},
		&Rule{},
		&SystemInfo{},
	)
	if err != nil {
		return nil, isNewDB, err
	}
	manager := &RepoManager{
		DB:          db,
		User:        NewUserRepo(db),
		UserGroup:   NewUserGroupRepo(db),
		ProxyData:   NewProxyDataRepo(db),
		RouteScheme: NewRouteSchemeRepo(db),
		Rule:        NewRuleRepo(db),
		SystemInfo:  NewSystemInfoRepo(db),
	}
	return manager, isNewDB, nil
}

func InitStatisticRepo(dbPath string) (*StatisticRepoManager, bool, error) {
	db, isNewDB, err := OpenDB(dbPath)
	if err != nil {
		return nil, isNewDB, err
	}

	// 迁移数据库表结构
	err = db.AutoMigrate(
		&Traffic{},
	)
	if err != nil {
		return nil, isNewDB, err
	}
	manager := &StatisticRepoManager{
		DB:      db,
		Traffic: NewTrafficRepo(db),
	}
	return manager, isNewDB, nil
}
