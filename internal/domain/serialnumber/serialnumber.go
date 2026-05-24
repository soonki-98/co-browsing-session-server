package serialnumber

type SerialNumber string

func (s SerialNumber) String() string {
	return string(s)
}
