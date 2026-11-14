package domain

type HoldStatus string

const (
	HoldActive   HoldStatus = "active"
	HoldExpired  HoldStatus = "expired"
	HoldCanceled HoldStatus = "canceled"
	HoldConsumed HoldStatus = "consumed"
)
