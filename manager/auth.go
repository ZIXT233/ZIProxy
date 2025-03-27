package manager

import (
	"github.com/ZIXT233/ziproxy/db"
)

func proxyAuth(info map[string]string) string {
	if userId, ok := info["username"]; ok {
		if password, ok := info["password"]; ok {
			if val, ok := UserMap.Load(userId); ok {
				if user, ok := val.(*db.User); ok {
					if user.Password == password {
						return userId
					}
				}
			}
		}
	}
	return "guest"
}
