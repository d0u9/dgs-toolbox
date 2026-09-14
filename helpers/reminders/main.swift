// dgs-reminders creates Apple Reminders for dgs. It exists because the
// Reminders scripting dictionary has no location alarm, and EventKit, which
// does, is not reachable from Go. It knows nothing about Captures: dgs decides
// what a reminder says and this only writes it.
//
//	dgs-reminders create < request.json   create one reminder
//	dgs-reminders lists                   print the reminder lists
//
// Every answer is one JSON object on stdout. A failure is {"error": "..."} and
// a non-zero exit, so dgs reports the reason rather than a bare status.
import CoreLocation
import EventKit
import Foundation

struct Location: Decodable {
    var title: String?
    var latitude: Double
    var longitude: Double
    // Metres. Absent leaves the system's own default radius.
    var radius: Double?
    // "arrive" or "leave".
    var proximity: String
}

struct CreateRequest: Decodable {
    var title: String
    var notes: String?
    // RFC 3339. Absent is a reminder with no due date.
    var due: String?
    // A list's title. Absent is the default list for new reminders.
    var list: String?
    // A token written into the notes and looked for before creating, so the
    // same Capture is not made into a reminder twice. Deleting the reminder is
    // how it is created again.
    var mark: String?
    var location: Location?
}

struct Failure: Error { let message: String }

func emit(_ object: [String: Any]) {
    let data = try! JSONSerialization.data(withJSONObject: object, options: [.sortedKeys])
    FileHandle.standardOutput.write(data)
    FileHandle.standardOutput.write("\n".data(using: .utf8)!)
}

func fail(_ message: String) -> Never {
    emit(["error": message])
    exit(1)
}

func requestAccess(_ store: EKEventStore) throws {
    let done = DispatchSemaphore(value: 0)
    var granted = false
    var failure: Error?
    let completion: (Bool, Error?) -> Void = { ok, error in
        granted = ok
        failure = error
        done.signal()
    }
    if #available(macOS 14.0, *) {
        store.requestFullAccessToReminders(completion: completion)
    } else {
        store.requestAccess(to: .reminder, completion: completion)
    }
    done.wait()
    if let failure { throw Failure(message: "reminders access: \(failure.localizedDescription)") }
    if !granted {
        throw Failure(message: "reminders access was denied; allow it in System Settings > Privacy & Security > Reminders")
    }
}

func calendar(_ store: EKEventStore, named list: String?) throws -> EKCalendar {
    guard let list, !list.isEmpty else {
        guard let calendar = store.defaultCalendarForNewReminders() else {
            throw Failure(message: "there is no default reminders list")
        }
        return calendar
    }
    let matches = store.calendars(for: .reminder).filter { $0.title == list }
    guard let calendar = matches.first else {
        throw Failure(message: "no reminders list is called \(list)")
    }
    if matches.count > 1 {
        throw Failure(message: "more than one reminders list is called \(list)")
    }
    return calendar
}

func existing(_ store: EKEventStore, in calendar: EKCalendar, mark: String) -> EKReminder? {
    let done = DispatchSemaphore(value: 0)
    var found: EKReminder?
    let predicate = store.predicateForReminders(in: [calendar])
    store.fetchReminders(matching: predicate) { reminders in
        found = reminders?.first { ($0.notes ?? "").contains(mark) }
        done.signal()
    }
    done.wait()
    return found
}

func parseDate(_ value: String) throws -> Date {
    let formatter = ISO8601DateFormatter()
    if let date = formatter.date(from: value) { return date }
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    if let date = formatter.date(from: value) { return date }
    throw Failure(message: "due \(value) is not an RFC 3339 timestamp")
}

func create(_ request: CreateRequest) throws -> [String: Any] {
    if request.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
        throw Failure(message: "a reminder needs a title")
    }
    let store = EKEventStore()
    try requestAccess(store)
    let target = try calendar(store, named: request.list)

    if let mark = request.mark, !mark.isEmpty, let found = existing(store, in: target, mark: mark) {
        return ["id": found.calendarItemIdentifier, "skipped": "this capture is already a reminder in \(target.title), as \(mark)"]
    }

    let reminder = EKReminder(eventStore: store)
    reminder.calendar = target
    reminder.title = request.title
    var notes = request.notes ?? ""
    if let mark = request.mark, !mark.isEmpty {
        notes = notes.isEmpty ? mark : notes + "\n\n" + mark
    }
    reminder.notes = notes.isEmpty ? nil : notes

    if let due = request.due {
        let date = try parseDate(due)
        var components = Calendar.current.dateComponents(
            [.year, .month, .day, .hour, .minute, .second, .timeZone], from: date)
        components.calendar = Calendar.current
        reminder.dueDateComponents = components
        reminder.addAlarm(EKAlarm(absoluteDate: date))
    }

    if let location = request.location {
        let structured = EKStructuredLocation(title: location.title ?? "")
        structured.geoLocation = CLLocation(latitude: location.latitude, longitude: location.longitude)
        if let radius = location.radius { structured.radius = radius }
        let alarm = EKAlarm()
        alarm.structuredLocation = structured
        switch location.proximity {
        case "arrive": alarm.proximity = .enter
        case "leave": alarm.proximity = .leave
        default: throw Failure(message: "proximity \(location.proximity) is neither arrive nor leave")
        }
        reminder.addAlarm(alarm)
    }

    do {
        try store.save(reminder, commit: true)
    } catch {
        throw Failure(message: "save the reminder: \(error.localizedDescription)")
    }
    return ["id": reminder.calendarItemIdentifier, "list": target.title]
}

func lists() throws -> [String: Any] {
    let store = EKEventStore()
    try requestAccess(store)
    let titles = store.calendars(for: .reminder).map { $0.title }
    return ["lists": titles, "default": store.defaultCalendarForNewReminders()?.title ?? ""]
}

let arguments = CommandLine.arguments.dropFirst()
do {
    switch arguments.first {
    case "create":
        let input = FileHandle.standardInput.readDataToEndOfFile()
        let request: CreateRequest
        do {
            request = try JSONDecoder().decode(CreateRequest.self, from: input)
        } catch {
            throw Failure(message: "the request is not valid: \(error)")
        }
        emit(try create(request))
    case "lists":
        emit(try lists())
    default:
        fail("usage: dgs-reminders create < request.json | dgs-reminders lists")
    }
} catch let failure as Failure {
    fail(failure.message)
} catch {
    fail(error.localizedDescription)
}
