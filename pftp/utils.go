package pftp

func safeSetChanel(c chan error, err error) {
	var ok bool
	select {
	case _, ok = <-c:
	default:
		ok = true
	}

	if ok {
		c <- err
	}
}
