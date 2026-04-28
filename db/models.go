package db

import (
	"database/sql"
	"time"
)

type User struct {
	ID            int64
	Username      string
	FullName      string
	Phone         string
	ReferredBy    int64
	ReferralCount int
	ReferralStatus int
	IsAdmin       bool
	IsActive      bool
	LastActive    time.Time
	CreatedAt     time.Time
}

func GetUser(id int64) (*User, error) {
	row := DB.QueryRow(`SELECT id, username, full_name, phone, referred_by, referral_count, referral_status, is_admin, is_active, last_active, created_at FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func GetUserByUsername(username string) (*User, error) {
	row := DB.QueryRow(`SELECT id, username, full_name, phone, referred_by, referral_count, referral_status, is_admin, is_active, last_active, created_at FROM users WHERE LOWER(username) = LOWER($1)`, username)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.FullName, &u.Phone, &u.ReferredBy, &u.ReferralCount, &u.ReferralStatus,
		&u.IsAdmin, &u.IsActive, &u.LastActive, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func UpsertUser(id int64, username, fullName string) error {
	_, err := DB.Exec(`
		INSERT INTO users (id, username, full_name, last_active)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			username    = excluded.username,
			full_name   = excluded.full_name,
			last_active = CURRENT_TIMESTAMP,
			is_active   = 1
	`, id, username, fullName)
	return err
}

func CreateUserWithReferral(id int64, username, fullName string, referredBy int64) error {
	_, err := DB.Exec(`
		INSERT INTO users (id, username, full_name, referred_by, referral_status, last_active)
		VALUES ($1, $2, $3, $4, 0, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			username    = excluded.username,
			full_name   = excluded.full_name,
			last_active = CURRENT_TIMESTAMP,
			is_active   = 1
	`, id, username, fullName, referredBy)
	return err
}

func ApproveReferral(userID int64) error {
	tx, err := DB.Begin()
	if err != nil { return err }

	var referredBy int64
	err = tx.QueryRow(`SELECT referred_by FROM users WHERE id = $1 AND referral_status = 0`, userID).Scan(&referredBy)
	if err != nil || referredBy == 0 {
		tx.Rollback()
		return err
	}
	
	res, err := tx.Exec(`UPDATE users SET referral_status = 1 WHERE id = $1 AND referral_status = 0`, userID)
	if err != nil { tx.Rollback(); return err }
	
	rows, _ := res.RowsAffected()
	if rows == 0 {
		tx.Rollback()
		return err
	}
	
	_, err = tx.Exec(`UPDATE users SET referral_count = referral_count + 1 WHERE id = $1`, referredBy)
	if err != nil { tx.Rollback(); return err }
	
	return tx.Commit()
}

func RevokeReferral(userID int64) error {
	var referredBy int64
	err := DB.QueryRow(`SELECT referred_by FROM users WHERE id = $1 AND referral_status = 1`, userID).Scan(&referredBy)
	if err != nil || referredBy == 0 {
		return err
	}
	
	tx, err := DB.Begin()
	if err != nil { return err }
	
	_, err = tx.Exec(`UPDATE users SET referral_status = -1 WHERE id = $1`, userID)
	if err != nil { tx.Rollback(); return err }
	
	_, err = tx.Exec(`UPDATE users SET referral_count = referral_count - 1 WHERE id = $1`, referredBy)
	if err != nil { tx.Rollback(); return err }
	
	return tx.Commit()
}

func UpdateUserPhone(id int64, phone string) error {
	_, err := DB.Exec(`UPDATE users SET phone = $1, last_active = CURRENT_TIMESTAMP WHERE id = $2`, phone, id)
	return err
}

func TouchUserActivity(id int64) error {
	_, err := DB.Exec(`UPDATE users SET last_active = CURRENT_TIMESTAMP, is_active = 1 WHERE id = $1`, id)
	return err
}

func UserExists(id int64) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM users WHERE id = $1`, id).Scan(&count)
	return count > 0, err
}

func GetTopReferrers(limit int) ([]User, error) {
	rows, err := DB.Query(`
		SELECT id, username, full_name, phone, referred_by, referral_count, referral_status, is_admin, is_active, last_active, created_at
		FROM users
		ORDER BY referral_count DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u := User{}
		err = rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Phone, &u.ReferredBy, &u.ReferralCount, &u.ReferralStatus,
			&u.IsAdmin, &u.IsActive, &u.LastActive, &u.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func DeactivateUser(id int64) error {
	_, err := DB.Exec(`UPDATE users SET is_active = 0 WHERE id = $1`, id)
	return err
}

type UserStats struct {
	Total    int
	Active   int
	Inactive int
}

func GetUserStats() (UserStats, error) {
	row := DB.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM users) as total,
			(SELECT COUNT(*) FROM users WHERE last_active >= NOW() - INTERVAL '30 days') as active,
			(SELECT COUNT(*) FROM users WHERE last_active < NOW() - INTERVAL '30 days') as inactive
	`)
	var s UserStats
	err := row.Scan(&s.Total, &s.Active, &s.Inactive)
	return s, err
}

func GetAllActiveUserIDs() ([]int64, error) {
	rows, err := DB.Query(`SELECT id FROM users WHERE is_active = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func IsAdmin(userID int64) bool {
	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM admins WHERE user_id = $1`, userID).Scan(&count)
	return count > 0
}

func AddAdmin(userID, addedBy int64) error {
	_, err := DB.Exec(`INSERT INTO admins (user_id, added_by) VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING`, userID, addedBy)
	if err != nil {
		return err
	}
	DB.Exec(`UPDATE users SET is_admin = 1 WHERE id = $1`, userID)
	return nil
}

func RemoveAdmin(userID int64) error {
	_, err := DB.Exec(`DELETE FROM admins WHERE user_id = $1`, userID)
	DB.Exec(`UPDATE users SET is_admin = 0 WHERE id = $1`, userID)
	return err
}

func GetAllAdmins() ([]User, error) {
	rows, err := DB.Query(`
		SELECT u.id, u.username, u.full_name, u.phone, u.referred_by, u.referral_count, u.referral_status, u.is_admin, u.is_active, u.last_active, u.created_at
		FROM admins a
		JOIN users u ON u.id = a.user_id
		ORDER BY a.added_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var admins []User
	for rows.Next() {
		u := User{}
		rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Phone, &u.ReferredBy, &u.ReferralCount, &u.ReferralStatus,
			&u.IsAdmin, &u.IsActive, &u.LastActive, &u.CreatedAt)
		admins = append(admins, u)
	}
	return admins, nil
}

type Channel struct {
	ID          int64
	ChannelID   string
	ChannelName string
	ChannelURL  string
	IsActive    bool
}

func GetActiveChannels() ([]Channel, error) {
	rows, err := DB.Query(`SELECT id, channel_id, channel_name, channel_url, is_active FROM channels WHERE is_active = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []Channel
	for rows.Next() {
		c := Channel{}
		rows.Scan(&c.ID, &c.ChannelID, &c.ChannelName, &c.ChannelURL, &c.IsActive)
		channels = append(channels, c)
	}
	return channels, nil
}

func GetAllChannels() ([]Channel, error) {
	rows, err := DB.Query(`SELECT id, channel_id, channel_name, channel_url, is_active FROM channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []Channel
	for rows.Next() {
		c := Channel{}
		rows.Scan(&c.ID, &c.ChannelID, &c.ChannelName, &c.ChannelURL, &c.IsActive)
		channels = append(channels, c)
	}
	return channels, nil
}

func AddChannel(channelID, channelName, channelURL string) error {
	_, err := DB.Exec(`INSERT INTO channels (channel_id, channel_name, channel_url) VALUES ($1, $2, $3) ON CONFLICT(channel_id) DO NOTHING`,
		channelID, channelName, channelURL)
	return err
}

func RemoveChannel(id int64) error {
	_, err := DB.Exec(`DELETE FROM channels WHERE id = $1`, id)
	return err
}

func ToggleChannel(id int64, active bool) error {
	val := 0
	if active {
		val = 1
	}
	_, err := DB.Exec(`UPDATE channels SET is_active = $1 WHERE id = $2`, val, id)
	return err
}

func GetSetting(key string) (string, error) {
	var val string
	err := DB.QueryRow(`SELECT value FROM bot_settings WHERE key = $1`, key).Scan(&val)
	return val, err
}

func SetSetting(key, value string) error {
	_, err := DB.Exec(`INSERT INTO bot_settings (key, value) VALUES ($1, $2)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}