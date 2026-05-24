package middleware

import (
	"co-browsing-session-server/internal/repository/session"

	"github.com/gin-gonic/gin"
)

const storeKey = "sessionStore"

func InjectSessionStore(store *session.SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(storeKey, store)
		c.Next()
	}
}

func GetSessionStore(c *gin.Context) *session.SessionStore {
	return c.MustGet(storeKey).(*session.SessionStore)
}
