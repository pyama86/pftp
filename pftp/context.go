package pftp

type Context struct {
	RemoteAddr string
}

func newContext(c *config) *Context {
	return &Context{
		RemoteAddr: c.Pftp.RemoteAddr,
	}
}
