// SPDX-License-Identifier: AGPL-3.0-only

package mount

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// capSysAdmin is the CAP_SYS_ADMIN bit number (linux/capability.h).
const capSysAdmin = 21

// capEffHasSysAdmin reads CapEff from the text of /proc/self/status and reports
// whether CAP_SYS_ADMIN is in the effective set. It returns an error, never a
// panic, for a missing or malformed line.
func capEffHasSysAdmin(status string) (bool, error) {
	for _, line := range strings.Split(status, "\n") {
		hexMask, ok := strings.CutPrefix(line, "CapEff:")
		if !ok {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(hexMask), 16, 64)
		if err != nil {
			return false, fmt.Errorf("CapEff is not a hex mask: %w", err)
		}
		return mask&(1<<capSysAdmin) != 0, nil
	}
	return false, errors.New("the process status has no CapEff line")
}

// checkCapability logs one error at startup when the process lacks CAP_SYS_ADMIN.
// It never stops the server: each mount then fails with the EPERM message
// (task 79, step 6). It does nothing outside Linux.
func (e *Executor) checkCapability() {
	if runtime.GOOS != "linux" {
		return
	}
	data, err := os.ReadFile(e.opt.ProcStatus)
	if err != nil {
		e.opt.Log.Warn("cannot read process capabilities; storage mounts may fail", "error", err)
		return
	}
	has, err := capEffHasSysAdmin(string(data))
	if err != nil {
		e.opt.Log.Warn("cannot read process capabilities; storage mounts may fail", "error", err)
		return
	}
	if !has {
		e.opt.Log.Error("network storage needs CAP_SYS_ADMIN, and this container does not have it. " +
			"Add cap_add: [SYS_ADMIN] and security_opt: [apparmor=unconfined] to the container. " +
			"Storage mounts will fail until then.")
	}
}
