package serialnumber

type SerialNumber string

func (serialNumber SerialNumber) String() string {
	return string(serialNumber)
}
