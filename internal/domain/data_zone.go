package domain

import (
	"strings"
	"time"
)

type DataZoneStatus string

const (
	DataZoneActive    DataZoneStatus = "active"
	DataZoneSuspended DataZoneStatus = "suspended"
)

type DataZone struct {
	ID         string         `json:"id"`
	Code       string         `json:"code"`
	Name       string         `json:"name"`
	Timezone   string         `json:"timezone"`
	Status     DataZoneStatus `json:"status"`
	DailyLimit int            `json:"daily_limit"`
	CutoffHour int            `json:"cutoff_hour"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Version    int64          `json:"version"`
}

func (s DataZone) Validate() error {
	if strings.TrimSpace(s.Code) == "" || strings.TrimSpace(s.Name) == "" {
		return FieldError{Field: "data_zone", Message: "code and name are required"}
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return FieldError{Field: "timezone", Message: "is invalid"}
	}
	if s.DailyLimit < 1 || s.DailyLimit > 10000 {
		return FieldError{Field: "daily_limit", Message: "must be between 1 and 10000"}
	}
	if s.CutoffHour < 0 || s.CutoffHour > 23 {
		return FieldError{Field: "cutoff_hour", Message: "must be between 0 and 23"}
	}
	if s.Status != DataZoneActive && s.Status != DataZoneSuspended {
		return FieldError{Field: "status", Message: "is invalid"}
	}
	return nil
}

func (s DataZone) BusinessDay(at time.Time) (string, error) {
	start, _, err := s.BusinessDayWindow(at)
	if err != nil {
		return "", err
	}
	loc, _ := time.LoadLocation(s.Timezone)
	return start.In(loc).Format("2006-01-02"), nil
}

func (s DataZone) BusinessDayWindow(at time.Time) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	local := at.In(loc)
	if local.Hour() < s.CutoffHour {
		local = local.AddDate(0, 0, -1)
	}
	start := time.Date(local.Year(), local.Month(), local.Day(), s.CutoffHour, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	return start.UTC(), end.UTC(), nil
}

func (s DataZone) IsOpen() bool { return s.Status == DataZoneActive }

func (s DataZone) IsSuspended() bool { return s.Status == DataZoneSuspended }
