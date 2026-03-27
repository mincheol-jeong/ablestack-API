package cube

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"os/exec"
	"strconv"
	"strings"
)

type TypeDisk struct {
	Target    string `json:"target"`
	Capacity  int    `json:"capacity"`
	Allocated int    `json:"allocated"`
	Physical  int    `json:"physical"`
}
type TypeNIC struct {
	Name       string `json:"name"`
	VName      string `json:"v_name"`
	MacAddress string `json:"mac_address"`
	Type       string `json:"type"`
	Source     string `json:"source"`
	Address    string `json:"address"`
	Link       string `json:"link"`
}
type TypeVMStatus struct {
	Name    string     `json:"name"`
	Running bool       `json:"running"`
	NIC     []TypeNIC  `json:"nic"`
	Disks   []TypeDisk `json:"disk"`
}

func GetVMStatus(vmname string) *TypeVMStatus {
	ret := &TypeVMStatus{
		Running: false,
		Name:    vmname,
		Disks:   make([]TypeDisk, 0),
	}
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte
		var output string
		var _ error
		var cmd *exec.Cmd

		// VM 동작상황 확인
		cmd = exec.Command("virsh", "domstate", vmname)

		stdout, _ = cmd.CombinedOutput()
		output = strings.TrimSpace(string(stdout))
		if output == "running" {
			ret.Running = true
		}

		// VM 디스크 목록
		cmd = exec.Command("virsh", "domblkinfo", vmname, "--all")

		stdout, _ = cmd.CombinedOutput()
		output = strings.TrimSpace(string(stdout))
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Target") || strings.HasPrefix(line, "--") {
				continue
			}
			fields := strings.Fields(line)
			Target := fields[0]
			Capacity, _ := strconv.Atoi(fields[1])
			Allocated, _ := strconv.Atoi(fields[2])
			Physical, _ := strconv.Atoi(fields[3])
			ret.Disks = append(ret.Disks, TypeDisk{Target: Target, Capacity: Capacity, Allocated: Allocated, Physical: Physical})
		}

		// VM nic 목록
		cmd = exec.Command("virsh", "domifaddr", vmname, "--full", "--source", "agent")

		stdout, _ = cmd.CombinedOutput()
		output = strings.TrimSpace(string(stdout))
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "--") || strings.HasPrefix(line, "lo") || strings.HasPrefix(line, "veth") {
				continue
			}
			fields := strings.Fields(line)
			Name := strings.TrimSpace(fields[0])
			MacAddress := strings.TrimSpace(fields[1])
			Address := strings.TrimSpace(fields[3])
			if fields[2] == "ipv6" {
				continue
			}
			cmd2 := exec.Command("virsh", "domif-getlink", vmname, MacAddress)

			stdout2, _ := cmd2.CombinedOutput()
			output2 := strings.TrimSpace(string(stdout2))
			field := strings.Fields(output2)
			Link := strings.TrimSpace(field[1])
			fmt.Println(output2)
			fmt.Println(field)

			cmd2 = exec.Command("virsh", "domiflist", vmname)

			stdout2, _ = cmd2.CombinedOutput()
			output2 = strings.TrimSpace(string(stdout2))
			lines2 := strings.Split(output2, "\n")
			Vname := "None"
			Type := "None"
			Source := "None"
			for _, line2 := range lines2 {
				line2 = strings.TrimSpace(line2)
				if strings.Contains(line2, MacAddress) {
					field2 := strings.Fields(line2)
					Vname = field2[0]
					Type = field2[1]
					Source = field2[2]
				}
			}

			Link = strings.TrimSpace(field[1])
			fmt.Println(output2)
			fmt.Println(field)

			ret.NIC = append(ret.NIC, TypeNIC{Name: Name, MacAddress: MacAddress, Address: Address, Link: Link, VName: Vname, Type: Type, Source: Source})
		}

	}
	jsonstr, err := json.Marshal(ret)
	fmt.Println(string(jsonstr))
	fmt.Println(err)
	return ret
}
