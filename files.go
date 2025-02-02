package virtualbox

import "github.com/iodasolutions/xbee-common/newfs"

func virtualboxFd() newfs.Folder {
	return newfs.GlobalXbeeFolder().ChildFolder("virtualbox")
}

func VolumesFolder() newfs.Folder {
	return virtualboxFd().ChildFolder("volumes")
}

func ExportFolder() newfs.Folder {
	return virtualboxFd().ChildFolder("exports")
}
