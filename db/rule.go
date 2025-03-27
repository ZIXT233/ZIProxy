package db

import "gorm.io/gorm"

type RuleRepo struct {
	db *gorm.DB
}

func NewRuleRepo(db *gorm.DB) *RuleRepo {
	return &RuleRepo{db: db}
}

func (r *RuleRepo) Create(rule *Rule) error {
	return r.db.Create(rule).Error
}
func (r *RuleRepo) GetByID(id uint) (*Rule, error) {
	var rule Rule
	result := r.db.Preload("RouteScheme").First(&rule, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &rule, nil
}

func (r *RuleRepo) Update(rule *Rule) error {
	return r.db.Save(rule).Error
}

func (r *RuleRepo) Delete(id uint) error {

	r.db.Model(&Rule{ID: id}).Association("Outbounds").Clear()

	return r.db.Delete(&Rule{}, id).Error
}

func (r *RuleRepo) ClearOutbounds(id uint) error {
	return r.db.Model(&Rule{ID: id}).Association("Outbounds").Clear()
}
