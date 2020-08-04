package player

func patchMediaType(t string) string {
	switch t {
	case "video/m4v":
		return "video/mp4"
	default:
		return t
	}
}
