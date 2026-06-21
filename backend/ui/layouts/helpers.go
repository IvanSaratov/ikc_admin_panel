package layouts

func navActive(active string, item string) string {
	if active == item {
		return "active"
	}
	return ""
}
