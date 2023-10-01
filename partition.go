package virtualbox

//type Partitions struct {
//	Vm *Vm
//}
//
//func (p *Partitions) notPartitionedDisks() (result []string) {
//	conn := p.Vm.Host.SSHConnexion()
//	out := conn.ExecuteCommandFail("sudo sfdisk -s")
//	scanner := bufio.NewScanner(strings.NewReader(out))
//	var allDisks []string
//	for scanner.Scan() {
//		line := scanner.Text()
//		if strings.HasPrefix(line, "/dev/sd") {
//			allDisks = append(allDisks, line[:strings.Index(line, ":")])
//		}
//	}
//	for _, disk := range allDisks {
//		if _, err := conn.ExecuteCommandNoFail(fmt.Sprintf("sudo fdisk -s  %s1", disk)); err != nil {
//			result = append(result, disk)
//		}
//	}
//	return
//}
//
//func (p *Partitions) EnsurePartitionsOK() {
//	for _, disk := range p.notPartitionedDisks() {
//		p.doPartitionDisk(disk)
//	}
//}
//
//func (p *Partitions) doPartitionDisk(disk string) {
//	p.Vm.dp.Infof("Create partition %s1", disk)
//	conn := p.Vm.Host.SSHConnexion()
//	command := fmt.Sprintf(`
//(
//echo o # Create a new empty DOS partition table
//echo n # Add a new partition
//echo p # Primary partition
//echo 1 # Partition number
//echo   # First sector (Accept default: 1)
//echo   # Last sector (Accept default: varies)
//echo w # Write changes
//) | sudo fdisk %s
//`, disk)
//	conn.ExecuteCommandFail(command)
//	conn.ExecuteCommandFail(fmt.Sprintf("sudo mkfs -t ext4 %s1", disk))
//}
