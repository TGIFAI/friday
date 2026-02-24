import Foundation
import IOKit
import IOKit.pwr_mgt

/// Manages macOS power assertions to prevent system sleep and lid-close sleep.
@MainActor
final class PowerManager: ObservableObject {
    @Published var preventSleep: Bool {
        didSet { updateSleepAssertion() }
    }
    @Published var preventLidSleep: Bool {
        didSet { updateLidAssertion() }
    }

    private var sleepAssertionID: IOPMAssertionID = 0
    private var lidAssertionID: IOPMAssertionID = 0

    private let defaults = UserDefaults.standard
    private static let sleepKey = "preventSleep"
    private static let lidKey = "preventLidSleep"

    init() {
        self.preventSleep = UserDefaults.standard.bool(forKey: Self.sleepKey)
        self.preventLidSleep = UserDefaults.standard.bool(forKey: Self.lidKey)
        // Apply restored state.
        updateSleepAssertion()
        updateLidAssertion()
    }

    // MARK: - Idle Sleep

    private func updateSleepAssertion() {
        defaults.set(preventSleep, forKey: Self.sleepKey)

        if preventSleep {
            guard sleepAssertionID == 0 else { return }
            let reason = "Friday is running — preventing idle sleep" as CFString
            let status = IOPMAssertionCreateWithName(
                kIOPMAssertionTypePreventUserIdleSystemSleep as CFString,
                IOPMAssertionLevel(kIOPMAssertionLevelOn),
                reason,
                &sleepAssertionID
            )
            if status != kIOReturnSuccess {
                sleepAssertionID = 0
            }
        } else {
            releaseSleepAssertion()
        }
    }

    private func releaseSleepAssertion() {
        guard sleepAssertionID != 0 else { return }
        IOPMAssertionRelease(sleepAssertionID)
        sleepAssertionID = 0
    }

    // MARK: - Lid-Close Sleep

    private func updateLidAssertion() {
        defaults.set(preventLidSleep, forKey: Self.lidKey)

        if preventLidSleep {
            guard lidAssertionID == 0 else { return }
            let reason = "Friday is running — preventing lid-close sleep" as CFString
            let status = IOPMAssertionCreateWithName(
                kIOPMAssertionTypePreventSystemSleep as CFString,
                IOPMAssertionLevel(kIOPMAssertionLevelOn),
                reason,
                &lidAssertionID
            )
            if status != kIOReturnSuccess {
                lidAssertionID = 0
            }
        } else {
            releaseLidAssertion()
        }
    }

    private func releaseLidAssertion() {
        guard lidAssertionID != 0 else { return }
        IOPMAssertionRelease(lidAssertionID)
        lidAssertionID = 0
    }

    // MARK: - Cleanup

    func releaseAll() {
        releaseSleepAssertion()
        releaseLidAssertion()
    }
}
