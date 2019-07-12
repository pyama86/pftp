package pftp

// Context struct got remote server address
type Context struct {
	RemoteAddr string
}

func newContext(c *config) *Context {
	return &Context{
		RemoteAddr: c.RemoteAddr,
	}
}
