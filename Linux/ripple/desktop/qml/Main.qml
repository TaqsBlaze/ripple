import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Dialogs
import "components"

ApplicationWindow {
    id: window
    visible: true
    width: 1180
    height: 760
    minimumWidth: 720
    minimumHeight: 480
    title: "Ripple"
    color: "#0a0a0a"
    property bool sidebarCollapsed: false
    readonly property bool compact: sidebarCollapsed || width < 950
    readonly property int pageMargin: compact ? 24 : 48
    property bool dropRippleActive: rippleService.dropActive || edgeDrop.containsDrag || dropArea.containsDrag
    property bool desktopDropActive: edgeDrop.containsDrag || dropArea.containsDrag
    onDesktopDropActiveChanged: rippleService.setDesktopDropActive(desktopDropActive)

    readonly property bool canSend: rippleService.transferReady && rippleService.trustedPeers.length === 1
    readonly property string targetId: canSend ? rippleService.trustedPeers[0].id : ""

    function sendFile(url) {
        if (canSend) rippleService.offerLocalFile(targetId, url)
    }

    FileDialog {
        id: sendFileDialog
        title: "Choose a file to send"
        fileMode: FileDialog.OpenFile
        onAccepted: window.sendFile(selectedFile)
    }

    FolderDialog {
        id: receiveFolderDialog
        title: "Choose where received files are saved"
        onAccepted: rippleService.setReceiveDir(selectedFolder)
    }

    property int page: 0
    readonly property color textPrimary: "#f5f5f5"
    readonly property color textSecondary: "#979797"
    readonly property color accent: "#ffffff"

    component Heading: Text {
        color: window.textPrimary
        font.pixelSize: 30
        font.weight: Font.DemiBold
    }

    component BodyText: Text {
        color: window.textSecondary
        font.pixelSize: 14
        wrapMode: Text.WordWrap
    }

    component PrimaryButton: Button {
        id: primaryButton
        implicitHeight: 44
        leftPadding: 20
        rightPadding: 20
        font.pixelSize: 14
        font.weight: Font.DemiBold
        contentItem: Text {
            text: primaryButton.text
            color: primaryButton.enabled ? "#111111" : "#686868"
            font: primaryButton.font
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
        }
        background: Rectangle {
            radius: 10
            color: primaryButton.enabled ? (primaryButton.down ? "#d7d7d7" : "#ffffff") : "#292929"
        }
    }

    RowLayout {
        anchors.fill: parent
        spacing: 0

        Rectangle {
            Layout.preferredWidth: window.compact ? 76 : 232
            Layout.fillHeight: true
            color: "#0d0d0d"
            border.color: "#202020"

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: window.compact ? 10 : 18
                spacing: 6

                RowLayout {
                    Layout.bottomMargin: 28
                    RippleMark { Layout.preferredWidth: 38; Layout.preferredHeight: 38 }
                    Text { visible: !window.compact; text: "ripple"; color: "white"; font.pixelSize: 23; font.weight: Font.DemiBold }
                    Item { Layout.fillWidth: true }
                }

                Button {
                    Layout.fillWidth: true
                    text: window.sidebarCollapsed ? "›" : "‹"
                    enabled: window.width >= 950
                    Accessible.name: window.sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"
                    ToolTip.visible: hovered
                    ToolTip.text: window.width < 950 ? "Sidebar expands when the window is wider" : (window.sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar")
                    onClicked: window.sidebarCollapsed = !window.sidebarCollapsed
                }

                NavItem { compact: window.compact; Layout.fillWidth: true; text: "Overview"; symbol: "⌂"; selected: window.page === 0; onClicked: window.page = 0 }
                NavItem { compact: window.compact; Layout.fillWidth: true; text: "Transfers"; symbol: "⇅"; selected: window.page === 1; onClicked: window.page = 1 }
                NavItem { compact: window.compact; Layout.fillWidth: true; text: "Assistant"; symbol: "✦"; selected: window.page === 2; onClicked: window.page = 2 }
                NavItem { compact: window.compact; Layout.fillWidth: true; text: "AI connections"; symbol: "◈"; selected: window.page === 3; onClicked: window.page = 3 }
                Item { Layout.fillHeight: true }
                NavItem { compact: window.compact; Layout.fillWidth: true; text: "Settings"; symbol: "⚙"; selected: window.page === 4; onClicked: window.page = 4 }

                Rectangle { Layout.fillWidth: true; height: 1; color: "#252525"; Layout.topMargin: 8; Layout.bottomMargin: 8 }
                RowLayout {
                    Layout.fillWidth: true
                    spacing: 10
                    Rectangle { width: 9; height: 9; radius: 5; color: rippleService.serviceOnline ? "#59d18a" : "#6d6d6d" }
                    ColumnLayout {
                        visible: !window.compact
                        spacing: 1
                        Text { text: rippleService.serviceOnline ? "Service connected" : "Service offline"; color: window.textPrimary; font.pixelSize: 12; font.weight: Font.Medium }
                        Text { text: rippleService.serviceVersion || "Not available"; color: window.textSecondary; font.pixelSize: 11 }
                    }
                }
            }
        }

        StackLayout {
            currentIndex: window.page
            Layout.fillWidth: true
            Layout.fillHeight: true

            Item {
                ScrollView {
                    anchors.fill: parent
                    contentWidth: availableWidth
                    contentHeight: pageContent.implicitHeight + pageContent.anchors.topMargin + window.pageMargin
                    ColumnLayout {
                        id: pageContent
                        width: Math.max(0, Math.min(820, parent.width - 2 * window.pageMargin))
                        anchors.horizontalCenter: parent.horizontalCenter
                        anchors.top: parent.top
                        anchors.topMargin: window.compact ? 24 : 52
                        spacing: 22

                        RowLayout {
                            Layout.fillWidth: true
                            ColumnLayout {
                                spacing: 6
                                Heading { text: "Your devices" }
                                BodyText { text: rippleService.statusMessage }
                            }
                            Item { Layout.fillWidth: true }
                            PrimaryButton { text: "+  Pair device"; enabled: rippleService.pairingReady; onClicked: rippleService.createInvitation() }
                        }

                        Panel {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 240
                            RowLayout { anchors.fill: parent; anchors.margins: 18; spacing: 18; Image { Layout.preferredWidth: 190; Layout.preferredHeight: 190; visible: rippleService.invitation.length > 0; source: rippleService.invitationQr; fillMode: Image.PreserveAspectFit; smooth: false; cache: false } ColumnLayout { Layout.fillWidth: true; Text { text: rippleService.invitation.length ? "Scan this code in Ripple on Android" : "Pairing code expired"; color: "white"; font.pixelSize: 14; font.weight: Font.DemiBold; wrapMode: Text.WordWrap } TextEdit { visible: rippleService.invitation.length > 0; Layout.fillWidth: true; Layout.fillHeight: true; readOnly: true; selectByMouse: true; text: rippleService.invitation; color: "#d0d0d0"; font.pixelSize: 10; font.family: "monospace"; wrapMode: TextEdit.WrapAnywhere } Text { text: rippleService.invitation.length ? "This code expires in two minutes. Keep this screen open while scanning." : "Generate a fresh code before scanning."; color: "#a7a7a7"; font.pixelSize: 11; wrapMode: Text.WordWrap } PrimaryButton { text: rippleService.invitation.length ? "Generate new code" : "Generate pairing code"; onClicked: rippleService.createInvitation() } } }
                        }
                        Repeater {
                            model: rippleService.pendingPairs
                            delegate: Panel {
                                required property var modelData
                                Layout.fillWidth: true
                                Layout.preferredHeight: 100
                                RowLayout { anchors.fill: parent; anchors.margins: 18; Text { Layout.fillWidth: true; text: modelData.name + " wants to pair"; color: "white"; font.pixelSize: 15 } Button { text: "Reject"; onClicked: rippleService.decidePairing(modelData.id,false) } PrimaryButton { text: "Approve"; onClicked: rippleService.decidePairing(modelData.id,true) } }
                            }
                        }

                        Panel {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 144
                            RowLayout {
                                anchors.fill: parent
                                anchors.margins: 22
                                spacing: 18
                                Rectangle {
                                    width: 58; height: 58; radius: 18; color: "#242424"
                                    Text { anchors.centerIn: parent; text: "▣"; color: "white"; font.pixelSize: 25 }
                                }
                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 6
                                    Text { text: rippleService.deviceName || "This PC"; color: window.textPrimary; font.pixelSize: 19; font.weight: Font.DemiBold }
                                    Row { spacing: 8; Rectangle { width: 8; height: 8; radius: 4; color: rippleService.serviceOnline ? "#59d18a" : "#666"; anchors.verticalCenter: parent.verticalCenter } Text { text: rippleService.serviceOnline ? "Private local service ready" : "Local service offline"; color: window.textSecondary; font.pixelSize: 13 } }
                                    Text { text: rippleService.fingerprint ? "Identity  " + rippleService.fingerprint.slice(0, 26) + "…" : "Identity unavailable"; color: "#737373"; font.pixelSize: 11; font.family: "monospace" }
                                }
                                Button { text: "Refresh"; onClicked: rippleService.refresh() }
                            }
                        }

                        Text { text: "Send files"; color: window.textPrimary; font.pixelSize: 17; font.weight: Font.DemiBold; Layout.topMargin: 8 }
                        Panel {
                            Layout.fillWidth: true
                            implicitHeight: sendContent.implicitHeight + 48
                            border.color: dropArea.containsDrag ? "#ffffff" : "#343434"
                            border.width: dropArea.containsDrag ? 2 : 1
                            DropArea {
                                id: dropArea
                                anchors.fill: parent
                                enabled: window.canSend
                                onEntered: function(drag) { if (!drag.hasUrls || drag.urls.length !== 1) drag.accepted = false }
                                onDropped: function(drop) {
                                    if (drop.hasUrls && drop.urls.length === 1) window.sendFile(drop.urls[0])
                                }
                            }
                            ColumnLayout {
                                id: sendContent
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.verticalCenter: parent.verticalCenter
                                width: Math.max(0, Math.min(460, parent.width - 48))
                                spacing: 12
                                Text { Layout.alignment: Qt.AlignHCenter; text: "⇧"; color: "white"; font.pixelSize: 30 }
                                Text { Layout.alignment: Qt.AlignHCenter; text: "Drop a file here"; color: window.textPrimary; font.pixelSize: 18; font.weight: Font.DemiBold }
                                BodyText {
                                    Layout.fillWidth: true
                                    horizontalAlignment: Text.AlignHCenter
                                    text: !rippleService.serviceOnline ? "Start the Ripple service to send files." :
                                          rippleService.trustedPeers.length === 0 ? "Pair an Android device to send files." :
                                          rippleService.trustedPeers.length > 1 ? "Sending requires exactly one paired Android device." :
                                          "Send to " + rippleService.trustedPeers[0].name + ". Check Ripple on Android to receive it."
                                }
                                PrimaryButton { Layout.alignment: Qt.AlignHCenter; text: "Choose file"; enabled: window.canSend; onClicked: sendFileDialog.open() }
                                BodyText { visible: rippleService.sendMessage.length > 0; Layout.fillWidth: true; horizontalAlignment: Text.AlignHCenter; text: rippleService.sendMessage }
                                Button { Layout.alignment: Qt.AlignHCenter; text: "View transfers"; onClicked: window.page = 1 }
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Text { text: "Recent transfers"; color: window.textPrimary; font.pixelSize: 17; font.weight: Font.DemiBold }
                            Item { Layout.fillWidth: true }
                            Text { text: "No verified receipts"; color: window.textSecondary; font.pixelSize: 12 }
                        }
                        Panel {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 108
                            ColumnLayout { anchors.centerIn: parent; spacing: 6; Text { Layout.alignment: Qt.AlignHCenter; text: "No transfers yet"; color: window.textPrimary; font.pixelSize: 15; font.weight: Font.Medium } BodyText { text: "Completed and failed transfers will appear here." } }
                        }
                    }
                }
            }

            Item {
                ScrollView {
                    anchors.fill: parent
                    contentWidth: availableWidth
                    contentHeight: transferContent.implicitHeight + transferContent.anchors.topMargin + window.pageMargin
                    ColumnLayout {
                        id: transferContent
                        width: Math.max(0, Math.min(820, parent.width - 2 * window.pageMargin))
                        anchors.horizontalCenter: parent.horizontalCenter
                        anchors.top: parent.top
                        anchors.topMargin: window.compact ? 24 : 52
                        spacing: 18
                        RowLayout {
                            Layout.fillWidth: true
                            Heading { text: "Transfers" }
                            Item { Layout.fillWidth: true }
                            BodyText { text: rippleService.transferReady ? "Receiving is ready" : "Service offline" }
                            Button { text: "Refresh"; onClicked: rippleService.refresh() }
                        }
                        Repeater {
                            model: rippleService.transfers
                            delegate: Panel {
                                required property var modelData
                                Layout.fillWidth: true
                                Layout.preferredHeight: modelData.state === "offered" ? 116 : 88
                                ColumnLayout {
                                    anchors.fill: parent
                                    anchors.margins: 18
                                    spacing: 8
                                    RowLayout {
                                        Layout.fillWidth: true
                                        ColumnLayout {
                                            Layout.fillWidth: true
                                            Text { text: modelData.name; color: window.textPrimary; font.pixelSize: 15; font.weight: Font.DemiBold; elide: Text.ElideMiddle; Layout.fillWidth: true }
                                            BodyText { text: (modelData.direction === "send" ? "To " : "From ") + modelData.peerName + " · " + Number(modelData.size).toLocaleString() + " bytes · " + (modelData.state === "completed" && modelData.direction === "send" ? "delivered" : modelData.state) }
                                        }
                                        Button { visible: modelData.direction !== "send" && modelData.state === "completed" && !modelData.deleted; text: "Open"; onClicked: rippleService.openReceivedFile(modelData.filePath) }
                                        Button { visible: modelData.direction !== "send" && modelData.state === "completed" && !modelData.deleted; text: "Delete"; onClicked: rippleService.deleteReceivedFile(modelData.id) }
                                        BodyText { visible: modelData.deleted; text: "File deleted" }
                                        BodyText { visible: modelData.state === "offered" && modelData.direction === "send"; text: "Waiting for Android to accept" }
                                        Button { visible: modelData.state === "offered" && modelData.direction !== "send"; text: "Reject"; onClicked: rippleService.decideTransfer(modelData.id, false) }
                                        PrimaryButton { visible: modelData.state === "offered" && modelData.direction !== "send"; text: "Accept"; onClicked: rippleService.decideTransfer(modelData.id, true) }
                                    }
                                    ProgressBar { visible: modelData.state === "transferring"; Layout.fillWidth: true; from: 0; to: Math.max(1, Number(modelData.size)); value: Number(modelData.offset) }
                                }
                            }
                        }
                        Panel {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 104
                            visible: rippleService.transfers.length === 0
                            ColumnLayout { anchors.centerIn: parent; spacing: 6; Text { Layout.alignment: Qt.AlignHCenter; text: "No transfers yet"; color: window.textPrimary; font.pixelSize: 15; font.weight: Font.Medium } BodyText { text: "Incoming file requests will appear here for approval." } }
                        }
                    }
                }
            }
            Item {
                ColumnLayout { anchors.centerIn: parent; width: Math.min(470, parent.width - 48); spacing: 12; Heading { Layout.alignment: Qt.AlignHCenter; text: "Assistant" } BodyText { Layout.fillWidth: true; horizontalAlignment: Text.AlignHCenter; text: "Ask this file becomes available after secure transfers. AI credentials will stay on this PC." } }
            }
            Item {
                ColumnLayout { anchors.centerIn: parent; width: Math.min(470, parent.width - 48); spacing: 12; Heading { Layout.alignment: Qt.AlignHCenter; text: "AI connections" } BodyText { Layout.fillWidth: true; horizontalAlignment: Text.AlignHCenter; text: "Local and cloud providers are not configured in this milestone. File transfer will work without AI." } }
            }
            Item {
                ScrollView {
                    anchors.fill: parent
                    contentWidth: availableWidth
                    contentHeight: settingsContent.implicitHeight + 48
                    ColumnLayout {
                    id: settingsContent
                    anchors.horizontalCenter: parent.horizontalCenter
                    anchors.top: parent.top
                    anchors.topMargin: 24
                    width: Math.min(620, parent.width - 48)
                    spacing: 18
                    Heading { text: "Settings" }
                    Panel {
                        Layout.fillWidth: true
                        implicitHeight: renameContent.implicitHeight + 44
                        ColumnLayout {
                            id: renameContent
                            anchors.fill: parent
                            anchors.margins: 22
                            spacing: 8
                            Text { text: "PC name"; color: window.textPrimary; font.pixelSize: 16; font.weight: Font.DemiBold }
                            BodyText { Layout.fillWidth: true; text: "Shown on your paired Android devices when they connect." }
                            RowLayout {
                                Layout.fillWidth: true
                                TextField {
                                    id: pcNameField
                                    Layout.fillWidth: true
                                    placeholderText: rippleService.deviceName
                                    maximumLength: 64
                                    enabled: rippleService.serviceOnline
                                    Accessible.name: "PC name"
                                    onAccepted: if (text.trim().length > 0) { rippleService.setDeviceName(text); text = "" }
                                }
                                PrimaryButton {
                                    text: "Save"
                                    enabled: rippleService.serviceOnline && pcNameField.text.trim().length > 0
                                    onClicked: { rippleService.setDeviceName(pcNameField.text); pcNameField.text = "" }
                                }
                            }
                            BodyText { Layout.fillWidth: true; text: "Current name: " + rippleService.deviceName }
                        }
                    }
                    Panel {
                        Layout.fillWidth: true
                        Layout.preferredHeight: Math.max(182, identityContent.implicitHeight + 44)
                        ColumnLayout {
                            id: identityContent
                            anchors.fill: parent
                            anchors.margins: 22
                            spacing: 8
                            Text { text: "This PC identity"; color: window.textPrimary; font.pixelSize: 16; font.weight: Font.DemiBold }
                            BodyText { text: "ECDSA P-256 public-key fingerprint" }
                            Text { Layout.fillWidth: true; text: rippleService.fingerprint || "Start the Ripple service to create the identity."; color: "#c7c7c7"; wrapMode: Text.WrapAnywhere; font.pixelSize: 12; font.family: "monospace" }
                            Item { Layout.fillHeight: true }
                            BodyText { text: "Private keys are stored in the user-only Ripple data directory and never enter QML." }
                        }
                    }
                    Panel {
                        Layout.fillWidth: true
                        implicitHeight: folderContent.implicitHeight + 44
                        ColumnLayout {
                            id: folderContent
                            anchors.fill: parent
                            anchors.margins: 22
                            spacing: 8
                            Text { text: "Receive folder"; color: window.textPrimary; font.pixelSize: 16; font.weight: Font.DemiBold }
                            BodyText { Layout.fillWidth: true; text: "New files from Android will be saved here. Existing files stay in their original folder." }
                            BodyText { Layout.fillWidth: true; text: rippleService.receiveDir || "Start the service to choose a folder."; wrapMode: Text.WrapAnywhere }
                            Button { text: "Choose folder"; enabled: rippleService.transferReady; onClicked: receiveFolderDialog.open() }
                            BodyText { Layout.fillWidth: true; visible: rippleService.folderMessage.length > 0; text: rippleService.folderMessage }
                        }
                    }
                    Panel {
                        Layout.fillWidth: true
                        Layout.preferredHeight: Math.max(108, settingsRow.implicitHeight + 44)
                        RowLayout {
                            id: settingsRow
                            anchors.fill: parent
                            anchors.margins: 22
                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: 4
                                Text { text: "Auto-accept incoming files"; color: window.textPrimary; font.pixelSize: 16; font.weight: Font.DemiBold }
                                BodyText { Layout.fillWidth: true; text: "Allow files from paired Android devices to start transferring without approval." }
                            }
                            Switch { checked: rippleService.autoAccept; onToggled: rippleService.setAutoAccept(checked) }
                        }
                    }
                    }
                }
            }
        }
    }

    Item {
        anchors.fill: parent
        z: 100
        visible: true
        DropArea {
            id: edgeDrop
            anchors.top: parent.top
            anchors.bottom: parent.bottom
            anchors.right: parent.right
            width: 54
            enabled: window.canSend
            onEntered: function(drag) { if (!drag.hasUrls || drag.urls.length !== 1) drag.accepted = false }
            onDropped: function(drop) {
                if (drop.hasUrls && drop.urls.length === 1) window.sendFile(drop.urls[0])
            }
            Rectangle {
                anchors.fill: parent
                visible: edgeDrop.containsDrag
                color: "#181a1dcc"
                border.color: "#f5f5f5"
                border.width: 1
                Text {
                    anchors.centerIn: parent
                    rotation: -90
                    text: "DROP TO SEND"
                    color: "white"
                    font.pixelSize: 11
                    font.weight: Font.DemiBold
                }
            }
        }
        Image {
            id: dropWave
            anchors.top: parent.top
            anchors.bottom: parent.bottom
            anchors.right: parent.right
            width: Math.min(window.width * 0.76, 420)
            source: "qrc:/assets/ripple-wave.svg"
            fillMode: Image.Stretch
            mirror: true
            opacity: window.dropRippleActive ? 0.92 : 0
            transformOrigin: Item.Right
            transform: Scale { id: dropWaveScale; origin.x: dropWave.width; origin.y: dropWave.height / 2; xScale: 1; yScale: 1 }
            Behavior on opacity {
                NumberAnimation { duration: window.dropRippleActive ? 220 : 1350; easing.type: Easing.OutCubic }
            }
            SequentialAnimation {
                running: window.dropRippleActive
                loops: Animation.Infinite
                NumberAnimation { target: dropWaveScale; property: "xScale"; from: 0.96; to: 1.04; duration: 2800; easing.type: Easing.InOutSine }
                NumberAnimation { target: dropWaveScale; property: "xScale"; from: 1.04; to: 0.96; duration: 2800; easing.type: Easing.InOutSine }
            }
        }
    }
}
