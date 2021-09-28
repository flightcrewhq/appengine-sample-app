package database

import "time"

const (
	userTable = "Users"
	docsTable = "Documents"
)

type User struct {
	ID        string
	Followers []string // datastore doesn't support maps
	Following []string

	Documents []int64

	Logins    int64
	LastLogin time.Time
}

type Document struct {
	ID          int64
	Author      string
	PublishTime time.Time
	Text        string `datastore:",noindex"`
}

type FollowRequest struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
}

type PublishRequest struct {
	User string `json:"user"`
	Text string `json:"text"`
}

func NewUser(id string) User {
	return User{
		ID:        id,
		Followers: make([]string, 0),
		Following: make([]string, 0),
		Documents: make([]int64, 0),
		Logins:    1,
		LastLogin: time.Now(),
	}
}

func (u *User) AddFollower(user string) {
	if !sliceContainsStr(user, u.Followers) {
		u.Followers = append(u.Followers, user)
	}
}

func (u *User) AddFollowing(user string) {
	if !sliceContainsStr(user, u.Following) {
		u.Following = append(u.Following, user)
	}
}

func (u *User) AddDocument(doc int64) {
	if !sliceContainsInt(doc, u.Documents) {
		u.Documents = append(u.Documents, doc)
	}
}

func sliceContainsStr(s string, slice []string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func sliceContainsInt(x int64, slice []int64) bool {
	for _, v := range slice {
		if v == x {
			return true
		}
	}
	return false
}
