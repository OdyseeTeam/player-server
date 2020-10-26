package player

func patchMediaType(t string) string {
	switch t {
	case "video/m4v", "video/webm":
		return "video/mp4"
	default:
		return t
	}
}
