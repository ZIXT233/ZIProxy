package db

import (
	"errors"

	"gorm.io/gorm"
)

type UserGroupRepo struct {
	db *gorm.DB
}

func NewUserGroupRepo(db *gorm.DB) *UserGroupRepo {
	return &UserGroupRepo{db: db}
}

func (r *UserGroupRepo) Create(group *UserGroup) error {
	return r.db.Create(group).Error
}

func (r *UserGroupRepo) GetByID(id string) (*UserGroup, error) {
	var group UserGroup
	result := r.db.Preload("RouteScheme").
		Preload("Users").
		Preload("AvailInbounds").
		First(&group, &UserGroup{ID: id})
	if result.Error != nil {
		return nil, result.Error
	}
	return &group, nil
}

func (r *UserGroupRepo) Update(group *UserGroup) error {
	return r.db.Save(group).Error
}

func (r *UserGroupRepo) Delete(id string) error {
	userGroup, err := r.GetByID(id)
	if err != nil {
		return err
	}
	//检查是否有关联用户
	count := r.db.Model(userGroup).Association("Users").Count()
	if count > 0 {
		return errors.New("cannot delete user group while it has associated users")
	}
	//检查是否有关联inbound
	r.db.Model(userGroup).Association("AvailInbounds").Clear()
	return r.db.Delete(userGroup).Error
}

func (r *UserGroupRepo) List(page, pageSize int) ([]UserGroup, int64, error) {
	var groups []UserGroup
	var total int64

	r.db.Model(&UserGroup{}).Count(&total)

	offset := (page - 1) * pageSize
	result := r.db.Preload("RouteScheme").
		Preload("Users").
		Preload("AvailInbounds").
		Offset(offset).Limit(pageSize).Find(&groups)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return groups, total, nil
}

func (r *UserGroupRepo) ClearInbounds(id string) error {
	userGroup, err := r.GetByID(id)
	if err != nil {
		return err
	}
	return r.db.Model(userGroup).Association("AvailInbounds").Clear()
}
