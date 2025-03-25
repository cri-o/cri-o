package watchdog_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/internal/watchdog"
	systemdmock "github.com/cri-o/cri-o/test/mocks/systemd"
)

// The actual test suite.
var _ = t.Describe("Watchdog", func() {
	const validTimeout = 4 * time.Second

	var (
		errTest     = errors.New("test")
		ctx         = context.Background()
		sut         *watchdog.Watchdog
		systemdMock *systemdmock.MockSystemd
	)

	waitForNotifications := func(x uint64) {
		for {
			if sut.Notifications() >= x {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}

	BeforeEach(func() {
		sut = watchdog.New()
		mockCtrl := gomock.NewController(GinkgoT())
		systemdMock = systemdmock.NewMockSystemd(mockCtrl)
		sut.SetSystemd(systemdMock)
	})

	It("should succeed if health checkers succeed", func() {
		// Given
		check1 := false
		check2 := false
		sut = watchdog.New(func(context.Context, time.Duration) error {
			check1 = true

			return nil
		}, func(context.Context, time.Duration) error {
			check2 = true

			return nil
		})
		sut.SetSystemd(systemdMock)

		gomock.InOrder(
			systemdMock.EXPECT().WatchdogEnabled().Return(validTimeout, nil),
			systemdMock.EXPECT().Notify(gomock.Any()).Return(true, nil),
		)

		// When
		err := sut.Start(ctx)
		waitForNotifications(1)

		// Then
		Expect(err).NotTo(HaveOccurred())
		Expect(check1).To(BeTrue())
		Expect(check2).To(BeTrue())
	})

	It("should retry if systemd notify fails", func() {
		// Given
		gomock.InOrder(
			systemdMock.EXPECT().WatchdogEnabled().Return(validTimeout, nil),
			systemdMock.EXPECT().Notify(gomock.Any()).Return(false, errTest),
			systemdMock.EXPECT().Notify(gomock.Any()).Return(true, nil),
		)

		// When
		err := sut.Start(ctx)
		waitForNotifications(2)

		// Then
		Expect(err).NotTo(HaveOccurred())
	})

	It("should abort if systemd doest not acknowledge", func() {
		// Given
		gomock.InOrder(
			systemdMock.EXPECT().WatchdogEnabled().Return(validTimeout, nil),
			systemdMock.EXPECT().Notify(gomock.Any()).Return(false, nil),
		)

		// When
		err := sut.Start(ctx)
		waitForNotifications(1)

		// Then
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not notify if one check is unhealthy", func() {
		// Given
		check1 := false
		check2 := false
		sut = watchdog.New(func(context.Context, time.Duration) error {
			check1 = true

			return nil
		}, func(context.Context, time.Duration) error {
			return errTest
		}, func(context.Context, time.Duration) error {
			check2 = true

			return nil
		})
		sut.SetSystemd(systemdMock)

		gomock.InOrder(
			systemdMock.EXPECT().WatchdogEnabled().Return(validTimeout, nil),
		)

		// When
		err := sut.Start(ctx)

		// Wait for check1 to become true
		for check1 != true {
			time.Sleep(time.Millisecond)
		}

		// Then
		Expect(err).NotTo(HaveOccurred())
		Expect(check1).To(BeTrue())
		Expect(check2).To(BeFalse())
		Expect(sut.Notifications()).To(BeZero())
	})

	It("should succeed with disabled systemd watchdog", func() {
		// Given
		gomock.InOrder(
			systemdMock.EXPECT().WatchdogEnabled().Return(time.Duration(0), nil),
		)

		// When
		err := sut.Start(ctx)

		// Then
		Expect(err).NotTo(HaveOccurred())
	})

	It("should fail if WatchdogEnabled fails", func() {
		// Given
		gomock.InOrder(
			systemdMock.EXPECT().WatchdogEnabled().Return(time.Duration(0), errTest),
		)

		// When
		err := sut.Start(ctx)

		// Then
		Expect(err).To(HaveOccurred())
	})

	It("should fail with too low interval", func() {
		// Given
		gomock.InOrder(
			systemdMock.EXPECT().WatchdogEnabled().Return(time.Millisecond, nil),
		)

		// When
		err := sut.Start(ctx)

		// Then
		Expect(err).To(HaveOccurred())
	})
})
