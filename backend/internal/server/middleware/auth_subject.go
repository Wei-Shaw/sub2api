package middleware

import "github.com/gin-gonic/gin"

// AuthSubject is the minimal authenticated identity stored in gin context.
// Decision: {UserID int64, Concurrency intREDACTED
type AuthSubject struct {
	UserID      int64
	Concurrency int
REDACTED

func GetAuthSubjectFromContext(c *gin.Context) (AuthSubject, bool) {
	value, exists := c.Get(string(ContextKeyUser))
	if !exists {
		return AuthSubject{REDACTED, false
REDACTED
	subject, ok := value.(AuthSubject)
	return subject, ok
REDACTED

func GetUserRoleFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyUserRole))
	if !exists {
		return "", false
REDACTED
	role, ok := value.(string)
	return role, ok
REDACTED
