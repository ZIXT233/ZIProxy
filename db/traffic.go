package db

import (
	"sort"
	"time"

	"gorm.io/gorm"
)

type TrafficRepo struct {
	db *gorm.DB
}

func NewTrafficRepo(db *gorm.DB) *TrafficRepo {
	return &TrafficRepo{db: db}
}

func (r *TrafficRepo) Create(traffic *Traffic) error {
	return r.db.Create(traffic).Error
}

func (r *TrafficRepo) GetByID(id uint) (*Traffic, error) {
	var traffic Traffic
	result := r.db.
		First(&traffic, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &traffic, nil
}

func (r *TrafficRepo) Update(traffic *Traffic) error {
	return r.db.Save(traffic).Error
}

func (r *TrafficRepo) Delete(id uint) error {
	return r.db.Delete(&Traffic{}, id).Error
}

func (r *TrafficRepo) List(page, pageSize int) ([]Traffic, int64, error) {
	var traffics []Traffic
	var total int64

	r.db.Model(&Traffic{}).Count(&total)

	offset := (page - 1) * pageSize
	result := r.db.
		Offset(offset).Limit(pageSize).Find(&traffics)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return traffics, total, nil
}

// 根据用户ID获取流量记录
func (r *TrafficRepo) GetByUserID(userID string, page, pageSize int) ([]Traffic, int64, error) {
	var traffics []Traffic
	var total int64

	r.db.Model(&Traffic{}).Where("user_id = ?", userID).Count(&total)

	offset := (page - 1) * pageSize
	result := r.db.
		Where("user_id = ?", userID).
		Offset(offset).Limit(pageSize).
		Order("time DESC").
		Find(&traffics)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return traffics, total, nil
}

// 获取指定时间范围内的流量记录
func (r *TrafficRepo) GetByTimeRange(startTime, endTime time.Time, page, pageSize int) ([]Traffic, int64, error) {
	var traffics []Traffic
	var total int64

	r.db.Model(&Traffic{}).Where("time BETWEEN ? AND ?", startTime, endTime).Count(&total)

	offset := (page - 1) * pageSize
	result := r.db.
		Where("time BETWEEN ? AND ?", startTime, endTime).
		Offset(offset).Limit(pageSize).
		Order("time DESC").
		Find(&traffics)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return traffics, total, nil
}

// 指定时间范围内的流量统计
func (r *TrafficRepo) GetTrafficStats(startTime, endTime time.Time) (uint64, uint64, error) {
	type Result struct {
		TotalBytesIn  uint64
		TotalBytesOut uint64
	}
	var result Result
	err := r.db.Model(&Traffic{}).
		Select("SUM(bytes_in) as total_bytes_in, SUM(bytes_out) as total_bytes_out").
		Where("time BETWEEN ? AND ?", startTime, endTime).
		Scan(&result).Error
	if err != nil {
		return 0, 0, err
	}
	return result.TotalBytesIn, result.TotalBytesOut, nil
}

// 获取用户在指定时间范围内的流量统计
func (r *TrafficRepo) GetUserTrafficStats(userID string, startTime, endTime time.Time) (uint64, uint64, error) {
	type Result struct {
		TotalBytesIn  uint64
		TotalBytesOut uint64
	}
	var result Result

	err := r.db.Model(&Traffic{}).
		Select("SUM(bytes_in) as total_bytes_in, SUM(bytes_out) as total_bytes_out").
		Where("user_id = ? AND time BETWEEN ? AND ?", userID, startTime, endTime).
		Scan(&result).Error

	if err != nil {
		return 0, 0, err
	}

	return result.TotalBytesIn, result.TotalBytesOut, nil
}

// 获取所有用户在指定时间范围内的流量排行
func (r *TrafficRepo) GetUserTrafficRank(startTime, endTime time.Time) ([]map[string]interface{}, error) {
	type Result struct {
		UserID        string
		TotalBytesIn  uint64
		TotalBytesOut uint64
	}
	var results []Result

	err := r.db.Model(&Traffic{}).
		Select("user_id, SUM(bytes_in) as total_bytes_in, SUM(bytes_out) as total_bytes_out").
		Where("time BETWEEN ? AND ?", startTime, endTime).
		Group("user_id").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	stats := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		stats = append(stats, map[string]interface{}{
			"name":     r.UserID,
			"download": r.TotalBytesIn,
			"upload":   r.TotalBytesOut,
			"traffic":  r.TotalBytesIn + r.TotalBytesOut,
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i]["traffic"].(uint64) > stats[j]["traffic"].(uint64)
	})
	return stats, nil
}

func (r *TrafficRepo) GetProxyTrafficRank(direction string, startTime, endTime time.Time) ([]map[string]interface{}, error) {
	type Result struct {
		ProxyID       string
		TotalBytesIn  uint64
		TotalBytesOut uint64
	}
	var results []Result
	var err error
	if direction == InDir {
		err = r.db.Model(&Traffic{}).
			Select("inbound_id as proxy_id, SUM(bytes_in) as total_bytes_in, SUM(bytes_out) as total_bytes_out").
			Where("time BETWEEN ? AND ?", startTime, endTime).
			Group("inbound_id").
			Scan(&results).Error

	} else {
		err = r.db.Model(&Traffic{}).
			Select("outbound_id as proxy_id, SUM(bytes_in) as total_bytes_in, SUM(bytes_out) as total_bytes_out").
			Where("time BETWEEN ? AND ?", startTime, endTime).
			Group("outbound_id").
			Scan(&results).Error
	}
	if err != nil {
		return nil, err
	}

	stats := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		stats = append(stats, map[string]interface{}{
			"name":     r.ProxyID,
			"download": r.TotalBytesIn,
			"upload":   r.TotalBytesOut,
			"traffic":  r.TotalBytesIn + r.TotalBytesOut,
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i]["traffic"].(uint64) > stats[j]["traffic"].(uint64)
	})
	return stats, nil
}

func (r *TrafficRepo) Clean(beforeTime time.Time) error {
	return r.db.Where("time < ?", beforeTime).Delete(&Traffic{}).Error
}
