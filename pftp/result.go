package pftp

type result struct {
	code int
	msg  string
	err  error
	log  *logger
}

func (r *result) Response(handler *clientHandler) error {
	if r.log != nil && r.err != nil {
		r.log.err("command error response: %s", r.err)
	}

	if r.code != 0 {
		return handler.writeMessage(r.code, r.msg)
	}
	return nil
}
