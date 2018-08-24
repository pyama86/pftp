package pftp

type Context struct {
	RemoteAddr string
}

func newContext(c *pftpConfig) *Context {
	return &Context{
		RemoteAddr: c.RemoteAddr,
	}
}
