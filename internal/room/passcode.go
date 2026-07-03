package room

import "math/rand"

func (r *Room) generatePasscodeLocked() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	r.Password = string(b)
	return r.Password
}

func (r *Room) consumePasscodeLocked(code string) bool {
	if !r.Private || code == "" || r.Password == "" {
		return false
	}
	if code != r.Password {
		return false
	}
	r.Password = ""
	return true
}
