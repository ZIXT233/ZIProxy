package db

import "gorm.io/gorm"

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(user *User) error {
	return r.db.Create(user).Error
}

func (r *UserRepo) GetByID(id string) (*User, error) {
	var user User
	result := r.db.Preload("UserGroup").First(&user, &User{ID: id})
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepo) GetByEmail(email string) (*User, error) {
	var user User
	result := r.db.Preload("UserGroup").First(&user, &User{Email: email})
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

func (r *UserRepo) Update(user *User) error {
	return r.db.Save(user).Error
}

func (r *UserRepo) Delete(id string) error {
	return r.db.Delete(&User{ID: id}).Error
}

func (r *UserRepo) List(page, pageSize int) ([]User, int64, error) {
	var users []User
	var total int64

	r.db.Model(&User{}).Count(&total)

	offset := (page - 1) * pageSize
	result := r.db.Preload("UserGroup").Offset(offset).Limit(pageSize).Find(&users)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return users, total, nil
}

func (r *UserRepo) Enable(id string, enabled bool) error {
	return r.db.Model(&User{}).Where("id = ?", id).Update("enabled", enabled).Error
}
