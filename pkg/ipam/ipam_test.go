package ipam

import (
	"net"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/taku-k/ipdrawer/pkg/model"
	"github.com/taku-k/ipdrawer/pkg/storage"
	"github.com/taku-k/ipdrawer/pkg/utils/testutil"
)

func (m *IPManager) reserveTemporary(ip net.IP) {
	_, _ = m.redis.Client.Set(makeIPTempReserved(ip), 1, 24*time.Hour).Result()
}

func TestIPActivation(t *testing.T) {
	r, deferFunc := storage.NewTestRedis()
	defer deferFunc()

	m := NewTestIPManager(r)

	ctx := context.Background()

	pool := &model.Pool{
		Start: "10.0.0.1",
		End:   "10.0.0.254",
	}

	if err := m.Activate(ctx, []*model.Pool{pool}, &model.IPAddr{Ip: "10.0.0.1"}); err != nil {
		t.Fatalf("Got error: %v", err)
	}
	if err := m.Activate(ctx, []*model.Pool{pool}, &model.IPAddr{Ip: "10.0.0.4"}); err != nil {
		t.Fatalf("Got error: %v", err)
	}

	s := net.ParseIP(pool.Start)
	e := net.ParseIP(pool.End)
	zkey := makePoolUsedIPZset(s, e)
	cnt, err := r.Client.ZCard(zkey).Result()
	if err != nil {
		t.Errorf("Got error: %v", err)
	}
	if cnt != 2 {
		t.Errorf("Expected %d, but got %d", 2, cnt)
	}
}

func TestDrawIPSeq(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		pool     *model.Pool
		ips      []*model.IPAddr
		expected net.IP
		errmsg   string
	}{
		{
			pool: &model.Pool{
				Start: "10.0.0.1",
				End:   "10.0.0.254",
			},
			ips: []*model.IPAddr{
				{
					Ip:     "10.0.0.1",
					Status: model.IPAddr_ACTIVE,
				}, {
					Ip:     "10.0.0.3",
					Status: model.IPAddr_ACTIVE,
				},
			},
			expected: net.ParseIP("10.0.0.2"),
		},
		{
			pool: &model.Pool{
				Start: "10.0.0.1",
				End:   "10.0.0.254",
			},
			ips: []*model.IPAddr{
				{
					Ip:     "10.0.0.1",
					Status: model.IPAddr_ACTIVE,
				}, {
					Ip:     "10.0.0.2",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.3",
					Status: model.IPAddr_ACTIVE,
				}, {
					Ip:     "10.0.0.4",
					Status: model.IPAddr_ACTIVE,
				},
			},
			expected: net.ParseIP("10.0.0.5"),
		},
		{
			pool: &model.Pool{
				Start: "10.0.0.1",
				End:   "10.0.0.254",
			},
			ips: []*model.IPAddr{
				{
					Ip:     "10.0.0.1",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.2",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.3",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.4",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				},
			},
			expected: net.ParseIP("10.0.0.5"),
		},
		{
			pool: &model.Pool{
				Start: "10.0.0.1",
				End:   "10.0.0.254",
			},
			ips: []*model.IPAddr{
				{
					Ip:     "10.0.0.1",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.2",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.3",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.4",
					Status: model.IPAddr_ACTIVE,
				},
			},
			expected: net.ParseIP("10.0.0.5"),
		},
		{
			pool: &model.Pool{
				Start: "10.0.0.1",
				End:   "10.0.0.2",
			},
			ips: []*model.IPAddr{
				{
					Ip:     "10.0.0.1",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				}, {
					Ip:     "10.0.0.2",
					Status: model.IPAddr_TEMPORARY_RESERVED,
				},
			},
			errmsg: "Nothing IP to serve",
		},
	}

	for i, c := range testCases {
		r, deferFunc := storage.NewTestRedis()
		m := NewTestIPManager(r)

		for _, ip := range c.ips {
			switch ip.Status {
			case model.IPAddr_ACTIVE:
				m.Activate(ctx, []*model.Pool{c.pool}, ip)
			case model.IPAddr_TEMPORARY_RESERVED:
				m.reserveTemporary(net.ParseIP(ip.Ip))
			case model.IPAddr_RESERVED:
				m.Reserve(c.pool, net.ParseIP(ip.Ip))
			}
		}

		actual, err := m.DrawIP(ctx, c.pool, true, false)

		if c.errmsg == "" {
			if err != nil {
				t.Errorf("#%d: Got error: %#+v", i, err)
			}
			if !c.expected.Equal(actual) {
				t.Errorf("#%d: expected %#+v, but got %#+v", i, c.expected.String(), actual.String())
			}
		} else {
			if err == nil {
				t.Errorf("#%d: expected error message %sf", i, c.errmsg)
			}
			if !testutil.IsError(err, c.errmsg) {
				t.Errorf("#%d: expected %q, but got %#+v", i, c.errmsg, err)
			}
		}

		deferFunc()
	}
}

func TestDeactivateAfterActivating(t *testing.T) {
	r, deferFunc := storage.NewTestRedis()
	defer deferFunc()

	m := NewTestIPManager(r)

	ctx := context.Background()

	pool := &model.Pool{
		Start: "10.0.0.1",
		End:   "10.0.0.254",
	}

	ip := &model.IPAddr{
		Ip: "10.0.0.1",
	}

	m.Activate(ctx, []*model.Pool{pool}, ip)

	if err := m.Deactivate(ctx, []*model.Pool{pool}, ip); err != nil {
		t.Errorf("Failed deactivating: %#+v", err)
	}

	keys, _ := r.Client.Keys(makeIPTempReserved(net.ParseIP(ip.Ip))).Result()
	if len(keys) != 0 {
		t.Errorf("Deactivation should remove temporary reserved key")
	}
}

func TestActivateIPInSeveralPools(t *testing.T) {
	r, deferFunc := storage.NewTestRedis()
	defer deferFunc()

	m := NewTestIPManager(r)

	ctx := context.Background()

	pools := []*model.Pool{
		{
			Start: "10.0.0.1",
			End:   "10.0.0.254",
		},
		{
			Start: "10.0.0.30",
			End:   "10.0.0.50",
		},
	}

	ip := &model.IPAddr{
		Ip: "10.0.0.40",
	}

	err := m.Activate(ctx, pools, ip)
	if err != nil {
		t.Errorf("Activate(%v, %v) returns %#+v; want success", pools, ip, err)
	}
}

func TestDeactivateIPInSeveralPools(t *testing.T) {
	r, deferFunc := storage.NewTestRedis()
	defer deferFunc()

	m := NewTestIPManager(r)

	ctx := context.Background()

	pools := []*model.Pool{
		{
			Start: "10.0.0.1",
			End:   "10.0.0.254",
		},
		{
			Start: "10.0.0.30",
			End:   "10.0.0.50",
		},
	}

	ip := &model.IPAddr{
		Ip: "10.0.0.40",
	}

	m.Activate(ctx, pools, ip)

	if err := m.Deactivate(ctx, pools, ip); err != nil {
		t.Errorf("Deactivate(%v, %v) returns %#+v; want success", pools, ip, err)
	}
}

func TestCorrectDrawIPFromInclusivePools(t *testing.T) {
	r, deferFunc := storage.NewTestRedis()
	defer deferFunc()

	m := NewTestIPManager(r)

	ctx := context.Background()

	pools := []*model.Pool{
		{
			Start: "10.0.0.1",
			End:   "10.0.0.254",
		},
		{
			Start: "10.0.0.1",
			End:   "10.0.0.10",
		},
	}

	ip := &model.IPAddr{
		Ip: "10.0.0.1",
	}

	m.Activate(ctx, pools, ip)

	actual, err := m.DrawIP(ctx, pools[1], true, false)
	if err != nil {
		t.Errorf("DrawIP returns err(%v); want success", err)
	}
	if !actual.Equal(net.ParseIP("10.0.0.2")) {
		t.Errorf("DrawIP returns incorrect IP(%v); want 10.0.0.2", actual.String())
	}
}