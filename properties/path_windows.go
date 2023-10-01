package properties

func VboxPath() string {
	root := os.Getenv("VBOX_MSI_INSTALL_PATH")
	if root == "" {
		panic(fmt.Errorf("On windows, env VBOX_MSI_INSTALL_PATH should be set"))
	}
	rootDir := newfs.Folder(root)
	VBoxPath = rootDir.ChildFile("VBoxManage.exe").String()
}