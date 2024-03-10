package momoblob

import "path"

func TGZKey(id string) string {
	return path.Join(id, "upload.tgz")
}

func APKKey(id string) string {
	return path.Join(id, "app.apk")
}

func IPAKey(id string) string {
	return path.Join(id, "app.ipa")
}

func FullSizeImageKey(id string) string {
	return path.Join(id, "full-size-image.png")
}

func DisplayImageKey(id string) string {
	return path.Join(id, "display-image.png")
}
