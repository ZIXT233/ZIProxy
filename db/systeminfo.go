// crud
package db

import "gorm.io/gorm"

type SystemInfoRepo struct {
	db *gorm.DB
}

func NewSystemInfoRepo(db *gorm.DB) *SystemInfoRepo {
	return &SystemInfoRepo{db: db}
}

func (r *SystemInfoRepo) Create(systemInfo *SystemInfo) error {
	return r.db.Create(systemInfo).Error
}

func (r *SystemInfoRepo) Update(systemInfo *SystemInfo) error {
	return r.db.Save(systemInfo).Error
}

func (r *SystemInfoRepo) GetbyID(id uint) (*SystemInfo, error) {
	var systemInfo SystemInfo
	if err := r.db.First(&systemInfo, id).Error; err != nil {
		return nil, err
	}
	return &systemInfo, nil
}
