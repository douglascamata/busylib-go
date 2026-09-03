package statestream

import (
	"github.com/douglascamata/busylib-go/busybar"
	"github.com/douglascamata/busylib-go/statestream/pb"
)

// The functions in this file map protobuf enums to the string values the HTTP
// API uses, so stream data and busybar responses can be compared directly.
// Unknown values map to "".

var batteryStatus = map[pb.BatteryStatus]busybar.PowerState{
	pb.BatteryStatus_DISCHARGING: busybar.PowerDischarging,
	pb.BatteryStatus_CHARGING:    busybar.PowerCharging,
	pb.BatteryStatus_CHARGED:     busybar.PowerCharged,
}

var wifiConnectionStatus = map[pb.WifiConnectionStatus]busybar.WifiState{
	pb.WifiConnectionStatus_CONNECTED:     busybar.WifiConnected,
	pb.WifiConnectionStatus_CONNECTING:    busybar.WifiConnecting,
	pb.WifiConnectionStatus_DISCONNECTING: busybar.WifiDisconnecting,
	pb.WifiConnectionStatus_RECONNECTING:  busybar.WifiReconnecting,
}

var wifiSecurity = map[pb.WifiSecurity]busybar.WifiSecurityMethod{
	pb.WifiSecurity_UNKNOWN:   busybar.WifiUnsupported,
	pb.WifiSecurity_OPEN:      busybar.WifiOpen,
	pb.WifiSecurity_WPA:       busybar.WifiWPA,
	pb.WifiSecurity_WPA2:      busybar.WifiWPA2,
	pb.WifiSecurity_WEP:       busybar.WifiWEP,
	pb.WifiSecurity_WPA_WPA2:  busybar.WifiWPAWPA2,
	pb.WifiSecurity_WPA3:      busybar.WifiWPA3,
	pb.WifiSecurity_WPA2_WPA3: busybar.WifiWPA2WPA3,
}

var ipMethod = map[pb.IpConfigurationMethod]busybar.WifiIPMethod{
	pb.IpConfigurationMethod_DHCP:   busybar.WifiDHCP,
	pb.IpConfigurationMethod_STATIC: busybar.WifiStatic,
}

var ipProtocol = map[pb.IpProtocol]busybar.WifiIPType{
	pb.IpProtocol_IPV4: busybar.WifiIPv4,
	pb.IpProtocol_IPV6: busybar.WifiIPv6,
}

var matterStatus = map[pb.MatterCommissioningStatus]busybar.SmartHomePairingState{
	pb.MatterCommissioningStatus_NEVER_STARTED:          busybar.PairingNeverStarted,
	pb.MatterCommissioningStatus_STARTED:                busybar.PairingStarted,
	pb.MatterCommissioningStatus_COMPLETED_SUCCESSFULLY: busybar.PairingCompletedSuccessfully,
	pb.MatterCommissioningStatus_FAILED:                 busybar.PairingFailed,
}

var bleStatus = map[pb.ServiceStatus]busybar.BleState{
	pb.ServiceStatus_RESET:          busybar.BleReset,
	pb.ServiceStatus_INITIALIZATION: busybar.BleInitialization,
	pb.ServiceStatus_READY:          busybar.BleEnabled,
	pb.ServiceStatus_ADVERTISING:    busybar.BleEnabled,
	pb.ServiceStatus_CONNECTABLE:    busybar.BleConnectable,
	pb.ServiceStatus_CONNECTED:      busybar.BleConnected,
	pb.ServiceStatus_ERROR:          busybar.BleInternalError,
}

var updateEvent = map[pb.UpdateEvent]busybar.UpdateEvent{
	pb.UpdateEvent_SESSION_START:   busybar.UpdateEventSessionStart,
	pb.UpdateEvent_SESSION_STOP:    busybar.UpdateEventSessionStop,
	pb.UpdateEvent_ACTION_BEGIN:    busybar.UpdateEventActionBegin,
	pb.UpdateEvent_ACTION_DONE:     busybar.UpdateEventActionDone,
	pb.UpdateEvent_DETAIL_CHANGE:   busybar.UpdateEventDetailChange,
	pb.UpdateEvent_ACTION_PROGRESS: busybar.UpdateEventActionProgress,
	pb.UpdateEvent_EVENT_NONE:      busybar.UpdateEventNone,
}

var updateAction = map[pb.UpdateAction]busybar.UpdateAction{
	pb.UpdateAction_DOWNLOAD:             busybar.UpdateActionDownload,
	pb.UpdateAction_SHA_VERIFICATION:     busybar.UpdateActionShaVerification,
	pb.UpdateAction_UNPACK:               busybar.UpdateActionUnpack,
	pb.UpdateAction_INSTALLATION_PREPARE: busybar.UpdateActionPrepare,
	pb.UpdateAction_INSTALLATION_APPLY:   busybar.UpdateActionApply,
	pb.UpdateAction_ACTION_NONE:          busybar.UpdateActionNone,
}

var updateStatus = map[pb.UpdateStatus]busybar.UpdateResult{
	pb.UpdateStatus_OK:                                                busybar.UpdateOK,
	pb.UpdateStatus_BATTERY_LOW:                                       busybar.UpdateBatteryLow,
	pb.UpdateStatus_BUSY:                                              busybar.UpdateBusy,
	pb.UpdateStatus_DOWNLOAD_FAILURE:                                  busybar.UpdateDownloadFailure,
	pb.UpdateStatus_DOWNLOAD_ABORT:                                    busybar.UpdateDownloadAbort,
	pb.UpdateStatus_SHA_MISMATCH:                                      busybar.UpdateShaMismatch,
	pb.UpdateStatus_UNPACK_CREATE_STAGING_DIRECTORY_FAILURE:           busybar.UpdateUnpackStagingDirFailure,
	pb.UpdateStatus_UNPACK_ARCHIVE_OPEN_FAILURE:                       busybar.UpdateUnpackArchiveOpenFailure,
	pb.UpdateStatus_UNPACK_ARCHIVE_UNPACK_FAILURE:                     busybar.UpdateUnpackArchiveUnpackFailure,
	pb.UpdateStatus_INSTALLATION_PREPARE_MANIFEST_NOT_FOUND:           busybar.UpdateInstallManifestNotFound,
	pb.UpdateStatus_INSTALLATION_PREPARE_MANIFEST_INVALID:             busybar.UpdateInstallManifestInvalid,
	pb.UpdateStatus_INSTALLATION_PREPARE_SESSION_CONFIG_SETUP_FAILURE: busybar.UpdateInstallSessionConfigFailure,
	pb.UpdateStatus_INSTALLATION_PREPARE_POINTER_SETUP_FAILURE:        busybar.UpdateInstallPointerSetupFailure,
	pb.UpdateStatus_UNKNOWN_FAILURE:                                   busybar.UpdateUnknownFailure,
}

var checkEvent = map[pb.CheckEvent]busybar.UpdateCheckEvent{
	pb.CheckEvent_START: busybar.CheckEventStart,
	pb.CheckEvent_STOP:  busybar.CheckEventStop,
	pb.CheckEvent_NONE:  busybar.CheckEventNone,
}

// CheckError explains why no update is available.
type CheckError string

const (
	CheckErrorNotAvailable CheckError = "not_available"
	CheckErrorFailure      CheckError = "failure"
	CheckErrorIdle         CheckError = "idle"
)

var checkError = map[pb.CheckError]CheckError{
	pb.CheckError_NOT_AVAILABLE: CheckErrorNotAvailable,
	pb.CheckError_FAILURE:       CheckErrorFailure,
	pb.CheckError_IDLE:          CheckErrorIdle,
}

func BatteryStatus(v pb.BatteryStatus) busybar.PowerState { return batteryStatus[v] }
func WifiConnectionStatus(v pb.WifiConnectionStatus) busybar.WifiState {
	return wifiConnectionStatus[v]
}
func WifiSecurity(v pb.WifiSecurity) busybar.WifiSecurityMethod { return wifiSecurity[v] }
func IPMethod(v pb.IpConfigurationMethod) busybar.WifiIPMethod  { return ipMethod[v] }
func IPProtocol(v pb.IpProtocol) busybar.WifiIPType             { return ipProtocol[v] }
func MatterStatus(v pb.MatterCommissioningStatus) busybar.SmartHomePairingState {
	return matterStatus[v]
}
func BleStatus(v pb.ServiceStatus) busybar.BleState       { return bleStatus[v] }
func UpdateEvent(v pb.UpdateEvent) busybar.UpdateEvent    { return updateEvent[v] }
func UpdateAction(v pb.UpdateAction) busybar.UpdateAction { return updateAction[v] }
func UpdateStatus(v pb.UpdateStatus) busybar.UpdateResult { return updateStatus[v] }
func CheckEvent(v pb.CheckEvent) busybar.UpdateCheckEvent { return checkEvent[v] }
func CheckErrorReason(v pb.CheckError) CheckError         { return checkError[v] }
