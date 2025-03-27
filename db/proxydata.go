package db

import (
	"gorm.io/gorm"
)

type ProxyDataRepo struct {
	db *gorm.DB
}

func NewProxyDataRepo(db *gorm.DB) *ProxyDataRepo {
	return &ProxyDataRepo{db: db}
}

func (r *ProxyDataRepo) Create(proxy *ProxyData) error {
	return r.db.Create(proxy).Error
}

func (r *ProxyDataRepo) GetByID(id string) (*ProxyData, error) {
	var proxy ProxyData
	result := r.db.Preload("UserGroups").First(&proxy, &ProxyData{ID: id})
	if result.Error != nil {
		return nil, result.Error
	}
	return &proxy, nil
}

func (r *ProxyDataRepo) Update(proxy *ProxyData) error {
	return r.db.Save(proxy).Error
}

func (r *ProxyDataRepo) Delete(id string) error {
	r.db.Model(&ProxyData{ID: id}).Association("UserGroups").Clear()
	return r.db.Delete(&ProxyData{ID: id}).Error
}

func (r *ProxyDataRepo) List(direction string, page, pageSize int) ([]ProxyData, int64, error) {
	var proxies []ProxyData
	var total int64

	query := r.db.Model(&ProxyData{})
	query = query.Where("direction = ?", direction)

	query.Count(&total)

	offset := (page - 1) * pageSize
	result := query.Preload("UserGroups").Offset(offset).Limit(pageSize).Find(&proxies)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return proxies, total, nil
}

func (r *ProxyDataRepo) Enable(id string, enabled bool) error {
	return r.db.Model(&ProxyData{}).Where("id = ?", id).Update("enabled", enabled).Error
}
