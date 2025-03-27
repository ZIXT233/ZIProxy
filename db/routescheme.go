package db

import (
	"errors"

	"gorm.io/gorm"
)

type RouteSchemeRepo struct {
	db *gorm.DB
}

func NewRouteSchemeRepo(db *gorm.DB) *RouteSchemeRepo {
	return &RouteSchemeRepo{db: db}
}

func (r *RouteSchemeRepo) Create(scheme *RouteScheme) error {
	return r.db.Create(scheme).Error
}

func (r *RouteSchemeRepo) GetByID(id string) (*RouteScheme, error) {
	var scheme RouteScheme
	result := r.db.Preload("UserGroups").Preload("Rules.Outbounds").First(&scheme, &RouteScheme{ID: id})
	if result.Error != nil {
		return nil, result.Error
	}
	return &scheme, nil
}

func (r *RouteSchemeRepo) Update(scheme *RouteScheme) error {
	return r.db.Save(scheme).Error
}

func (r *RouteSchemeRepo) Delete(id string) error {
	var count int64
	r.db.Model(&UserGroup{}).Where("route_scheme_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("cannot delete route scheme while it's being used by user groups")
	}
	r.db.Model(&Rule{}).Where("route_scheme_id= ?", id).Delete(nil)
	return r.db.Delete(&RouteScheme{ID: id}).Error
}

func (r *RouteSchemeRepo) List(page, pageSize int) ([]RouteScheme, int64, error) {
	var schemes []RouteScheme
	var total int64

	r.db.Model(&RouteScheme{}).Count(&total)

	offset := (page - 1) * pageSize
	result := r.db.Preload("UserGroups").Preload("Rules.Outbounds").Offset(offset).Limit(pageSize).Find(&schemes)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return schemes, total, nil
}

func (r *RouteSchemeRepo) Enable(id string, enabled bool) error {
	return r.db.Model(&RouteScheme{}).Where("id = ?", id).Update("enabled", enabled).Error
}
