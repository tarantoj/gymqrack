import SwiftUI

struct LoginView: View {
    @EnvironmentObject private var controller: SessionController
    @State private var email = ""
    @State private var password = ""
    @FocusState private var focusedField: Field?

    private enum Field {
        case email
        case password
    }

    var body: some View {
        NavigationStack {
            Group {
                if controller.isSignedIn {
                    signedInView
                } else {
                    loginForm
                }
            }
            .navigationTitle("VivaGym Watch")
        }
    }

    private var loginForm: some View {
        Form {
            Section {
                TextField("Email", text: $email)
                    .textContentType(.username)
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .focused($focusedField, equals: .email)
                SecureField("Password", text: $password)
                    .textContentType(.password)
                    .focused($focusedField, equals: .password)
                    .submitLabel(.go)
            }

            Section {
                Button {
                    Task { await submit() }
                } label: {
                    if controller.isWorking {
                        ProgressView()
                    } else {
                        Text("Sign in")
                    }
                }
                .disabled(controller.isWorking || email.isEmpty || password.isEmpty)
            } footer: {
                Text("Your VivaGym email and password are sent straight to VivaGym; only tokens are stored on this device.")
            }

            if let message = controller.errorMessage {
                Section {
                    Text(message)
                        .foregroundStyle(.red)
                }
            }
        }
        .scrollDismissesKeyboard(.interactively)
    }

    private var signedInView: some View {
        Form {
            Section {
                LabeledContent("Signed in as", value: controller.email)
                LabeledContent("Clubs", value: "\(controller.clubs.count)")
            }

            if !controller.clubs.isEmpty {
                Section("Your clubs") {
                    ForEach(controller.clubs) { club in
                        VStack(alignment: .leading, spacing: 2) {
                            Text(club.clubName ?? "VivaGym")
                                .font(.subheadline)
                            Text([club.address1, club.address2, club.postalCode]
                                .compactMap { $0 }
                                .joined(separator: ", "))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }

            Section {
                Button("Sign out", role: .destructive) {
                    controller.signOut()
                }
            } footer: {
                Text("Open VivaGym on your watch: the entry QR appears when you're near your club.")
            }
        }
    }

    private func submit() async {
        focusedField = nil
        await controller.signIn(email: email, password: password)
        if controller.isSignedIn {
            email = ""
            password = ""
        }
    }
}

@main
struct CompanionApp: App {
    @StateObject private var controller = SessionController()

    var body: some Scene {
        WindowGroup {
            LoginView()
                .environmentObject(controller)
                .task {
                    SessionSync.shared.activate()
                }
        }
    }
}