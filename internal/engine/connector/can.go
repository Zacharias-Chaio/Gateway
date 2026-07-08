package connector

import (
	"context"
	"time"
)

// canDriver 是 CAN 链路的占位实现。第一阶段不接入 SocketCAN，
// 所有操作返回 ErrNotSupported；接口签名已就位，后续阶段可平滑替换。
type canDriver struct {
	name string
}

// newCANDriver 构造 CAN 占位驱动。
func newCANDriver(cfg Config) (Driver, error) {
	return &canDriver{name: cfg.CanName}, nil
}

func (d *canDriver) Open(_ context.Context) error { return ErrNotSupported }

func (d *canDriver) Send(_ []byte) (int, error) { return 0, ErrNotSupported }

func (d *canDriver) Receive(_ []byte, _ time.Duration) (int, error) { return 0, ErrNotSupported }

func (d *canDriver) Refresh() error { return ErrNotSupported }

func (d *canDriver) Close() error { return nil }

func (d *canDriver) Info() Info {
	return Info{Type: TypeCAN, Target: d.name, Open: false}
}
