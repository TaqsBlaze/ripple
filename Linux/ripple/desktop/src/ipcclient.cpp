#include "ipcclient.h"

#include <QDir>
#include <QDesktopServices>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QStandardPaths>
#include <QUrl>
#include <QUuid>

namespace {
constexpr quint32 MaxFrameSize = 1024U * 1024U;

QString socketPath()
{
    const QString runtime = QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
    return QDir(runtime).filePath(QStringLiteral("ripple/ripple.sock"));
}

quint32 readBigEndian(const QByteArray &bytes)
{
    return (static_cast<quint32>(static_cast<unsigned char>(bytes[0])) << 24U)
        | (static_cast<quint32>(static_cast<unsigned char>(bytes[1])) << 16U)
        | (static_cast<quint32>(static_cast<unsigned char>(bytes[2])) << 8U)
        | static_cast<quint32>(static_cast<unsigned char>(bytes[3]));
}

QByteArray prefixFrame(const QByteArray &payload)
{
    const quint32 size = static_cast<quint32>(payload.size());
    QByteArray frame(4, Qt::Uninitialized);
    frame[0] = static_cast<char>((size >> 24U) & 0xffU);
    frame[1] = static_cast<char>((size >> 16U) & 0xffU);
    frame[2] = static_cast<char>((size >> 8U) & 0xffU);
    frame[3] = static_cast<char>(size & 0xffU);
    frame.append(payload);
    return frame;
}
}

IpcClient::IpcClient(QObject *parent)
    : QObject(parent)
{
    connect(&m_socket, &QLocalSocket::connected, this, &IpcClient::connected);
    connect(&m_socket, &QLocalSocket::disconnected, this, &IpcClient::disconnected);
    connect(&m_socket, &QLocalSocket::readyRead, this, &IpcClient::readAvailable);
    connect(&m_socket, &QLocalSocket::errorOccurred, this, &IpcClient::socketError);
    m_retryTimer.setInterval(3000);
    connect(&m_retryTimer, &QTimer::timeout, this, &IpcClient::refresh);
    m_retryTimer.start();
    m_dropTimer.setInterval(150);
    connect(&m_dropTimer, &QTimer::timeout, this, [this] {
        if (m_socket.state() != QLocalSocket::ConnectedState) return;
        const QJsonObject request{{QStringLiteral("version"), 1}, {QStringLiteral("type"), QStringLiteral("drop.status")}, {QStringLiteral("requestId"), QUuid::createUuid().toString(QUuid::WithoutBraces)}};
        m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
        if (m_desktopDropActive) setDesktopDropActive(true);
    });
    m_invitationExpiry.setSingleShot(true);
    connect(&m_invitationExpiry,&QTimer::timeout,this,[this]{m_invitation.clear();m_invitationQr.clear();emit stateChanged();});
    QTimer::singleShot(0, this, &IpcClient::refresh);
}

void IpcClient::setDesktopDropActive(bool active)
{
    m_desktopDropActive = active;
    if (m_socket.state() != QLocalSocket::ConnectedState) return;
    const QJsonObject request{{QStringLiteral("version"), 1}, {QStringLiteral("type"), QStringLiteral("drop.desktop")}, {QStringLiteral("requestId"), QUuid::createUuid().toString(QUuid::WithoutBraces)}, {QStringLiteral("payload"), QJsonObject{{QStringLiteral("active"), active}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::refresh()
{
    if (m_socket.state() == QLocalSocket::ConnectedState) {
        requestStatus();
        return;
    }
    if (m_socket.state() == QLocalSocket::UnconnectedState)
        m_socket.connectToServer(socketPath(), QIODevice::ReadWrite);
}

void IpcClient::connected()
{
    m_serviceOnline = true;
    m_statusMessage = QStringLiteral("Local service connected");
    emit stateChanged();
    requestStatus();
    m_dropTimer.start();
}

void IpcClient::disconnected()
{
    setOffline(QStringLiteral("Ripple service is not running"));
}

void IpcClient::socketError(QLocalSocket::LocalSocketError error)
{
    Q_UNUSED(error)
    if (m_socket.state() != QLocalSocket::ConnectedState)
        setOffline(QStringLiteral("Start rippled to connect the desktop app"));
}

void IpcClient::requestStatus()
{
    const QJsonObject request{
        {QStringLiteral("version"), 1},
        {QStringLiteral("type"), QStringLiteral("service.status")},
        {QStringLiteral("requestId"), QUuid::createUuid().toString(QUuid::WithoutBraces)}
    };
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::createInvitation()
{
    const QJsonObject request{{QStringLiteral("version"),1},{QStringLiteral("type"),QStringLiteral("pairing.invitation")},{QStringLiteral("requestId"),QUuid::createUuid().toString(QUuid::WithoutBraces)}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::decidePairing(const QString &id, bool approve)
{
    const QJsonObject request{{QStringLiteral("version"),1},{QStringLiteral("type"),QStringLiteral("pairing.decide")},{QStringLiteral("requestId"),QUuid::createUuid().toString(QUuid::WithoutBraces)},{QStringLiteral("payload"),QJsonObject{{QStringLiteral("id"),id},{QStringLiteral("approve"),approve}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::decideTransfer(const QString &id, bool approve)
{
    const QJsonObject request{{QStringLiteral("version"),1},{QStringLiteral("type"),QStringLiteral("transfer.decide")},{QStringLiteral("requestId"),QUuid::createUuid().toString(QUuid::WithoutBraces)},{QStringLiteral("payload"),QJsonObject{{QStringLiteral("id"),id},{QStringLiteral("approve"),approve}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::setDeviceName(const QString &name)
{
    const QJsonObject request{{QStringLiteral("version"),1},{QStringLiteral("type"),QStringLiteral("device.rename")},{QStringLiteral("requestId"),QUuid::createUuid().toString(QUuid::WithoutBraces)},{QStringLiteral("payload"),QJsonObject{{QStringLiteral("name"),name}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::setReceiveDir(const QUrl &url)
{
    if (!url.isLocalFile()) { m_folderMessage = QStringLiteral("Choose a local folder."); emit stateChanged(); return; }
    m_folderMessage.clear();
    const QJsonObject request{{QStringLiteral("version"),1},{QStringLiteral("type"),QStringLiteral("transfer.receive_dir")},{QStringLiteral("requestId"),QUuid::createUuid().toString(QUuid::WithoutBraces)},{QStringLiteral("payload"),QJsonObject{{QStringLiteral("path"),url.toLocalFile()}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::setAutoAccept(bool enabled)
{
    const QJsonObject request{{QStringLiteral("version"),1},{QStringLiteral("type"),QStringLiteral("transfer.auto_accept")},{QStringLiteral("requestId"),QUuid::createUuid().toString(QUuid::WithoutBraces)},{QStringLiteral("payload"),QJsonObject{{QStringLiteral("enabled"),enabled}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

bool IpcClient::openReceivedFile(const QString &path)
{
    const QFileInfo file(path);
    if (!file.isFile() || !QDesktopServices::openUrl(QUrl::fromLocalFile(file.absoluteFilePath()))) {
        m_statusMessage = QStringLiteral("Could not open this received file. Check that a default app is installed for its file type.");
        emit stateChanged();
        return false;
    }
    return true;
}

void IpcClient::deleteReceivedFile(const QString &id)
{
    const QJsonObject request{{QStringLiteral("version"), 1}, {QStringLiteral("type"), QStringLiteral("transfer.delete")}, {QStringLiteral("requestId"), QUuid::createUuid().toString(QUuid::WithoutBraces)}, {QStringLiteral("payload"), QJsonObject{{QStringLiteral("id"), id}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::offerLocalFile(const QString &peerId, const QUrl &url)
{
    if (!url.isLocalFile() || url.toLocalFile().isEmpty()) {
        m_sendMessage = QStringLiteral("Choose a local file to send.");
        emit stateChanged();
        return;
    }
    const QString path = url.toLocalFile();
    m_sendMessage = QStringLiteral("Preparing %1…").arg(QFileInfo(path).fileName());
    emit stateChanged();
    const QJsonObject request{{QStringLiteral("version"), 1}, {QStringLiteral("type"), QStringLiteral("transfer.offer_local")}, {QStringLiteral("requestId"), QUuid::createUuid().toString(QUuid::WithoutBraces)}, {QStringLiteral("payload"), QJsonObject{{QStringLiteral("peerId"), peerId}, {QStringLiteral("path"), path}}}};
    m_socket.write(prefixFrame(QJsonDocument(request).toJson(QJsonDocument::Compact)));
}

void IpcClient::readAvailable()
{
    m_buffer.append(m_socket.readAll());
    while (m_buffer.size() >= 4) {
        const quint32 size = readBigEndian(m_buffer.first(4));
        if (size == 0 || size > MaxFrameSize) {
            m_socket.abort();
            setOffline(QStringLiteral("The local service sent an invalid response"));
            return;
        }
        if (m_buffer.size() < static_cast<int>(size + 4U))
            return;
        const QByteArray frame = m_buffer.mid(4, static_cast<int>(size));
        m_buffer.remove(0, static_cast<int>(size + 4U));
        handleFrame(frame);
    }
}

void IpcClient::handleFrame(const QByteArray &frame)
{
    QJsonParseError error;
    const QJsonDocument document = QJsonDocument::fromJson(frame, &error);
    const QJsonObject response = document.object();
    if (error.error != QJsonParseError::NoError || response.value(QStringLiteral("version")).toInt() != 1) {
        m_statusMessage = QStringLiteral("Desktop and service versions are incompatible");
        emit stateChanged();
        return;
    }
    if (response.contains(QStringLiteral("error"))) {
        m_statusMessage = response.value(QStringLiteral("error")).toObject().value(QStringLiteral("message")).toString();
        if (response.value(QStringLiteral("type")).toString() == QStringLiteral("transfer.receive_dir.result")) m_folderMessage = m_statusMessage;
        if (response.value(QStringLiteral("type")).toString() == QStringLiteral("transfer.offer_local.result"))
            m_sendMessage = m_statusMessage;
        emit stateChanged();
        return;
    }
    const QJsonObject payload = response.value(QStringLiteral("payload")).toObject();
    if (response.value(QStringLiteral("type")).toString() == QStringLiteral("pairing.invitation.result")) {
        m_invitation = payload.value(QStringLiteral("invitation")).toString();
        m_invitationQr = payload.value(QStringLiteral("qrData")).toString();
        m_invitationExpiry.start(118000);
        emit stateChanged();
        return;
    }
    const QString responseType = response.value(QStringLiteral("type")).toString();
    if (responseType == QStringLiteral("transfer.offer_local.result"))
        m_sendMessage = QStringLiteral("%1 is ready. Check Ripple on Android.").arg(payload.value(QStringLiteral("name")).toString());
    if (responseType == QStringLiteral("transfer.receive_dir.result")) { m_folderMessage = QStringLiteral("Receive folder updated."); emit stateChanged(); QTimer::singleShot(0, this, &IpcClient::refresh); return; }
    if (responseType == QStringLiteral("drop.desktop.result")) return;
    if (responseType == QStringLiteral("drop.status.result")) {
        const bool active = payload.value(QStringLiteral("active")).toBool();
        if (m_dropActive != active) { m_dropActive = active; emit stateChanged(); }
        return;
    }
    if (responseType == QStringLiteral("device.rename.result") || responseType == QStringLiteral("transfer.decide.result") || responseType == QStringLiteral("transfer.auto_accept.result") || responseType == QStringLiteral("transfer.delete.result") || responseType == QStringLiteral("transfer.offer_local.result")) {
        emit stateChanged();
        QTimer::singleShot(0, this, &IpcClient::refresh);
        return;
    }
    m_serviceVersion = payload.value(QStringLiteral("serviceVersion")).toString();
    m_deviceName = payload.value(QStringLiteral("deviceName")).toString(QStringLiteral("This PC"));
    m_fingerprint = payload.value(QStringLiteral("identityFingerprint")).toString();
    m_pairingReady = payload.value(QStringLiteral("pairingReady")).toBool();
    m_transferReady = payload.value(QStringLiteral("transferReady")).toBool();
    m_pendingPairs = payload.value(QStringLiteral("pendingPairs")).toArray();
    m_trustedPeers = payload.value(QStringLiteral("trustedPeers")).toArray();
    m_transfers = payload.value(QStringLiteral("transfers")).toArray();
    m_receiveDir = payload.value(QStringLiteral("receiveDir")).toString();
    m_autoAccept = payload.value(QStringLiteral("autoAccept")).toBool();
    m_serviceOnline = true;
    m_statusMessage = m_pairingReady
        ? QStringLiteral("Ready for nearby devices")
        : QStringLiteral("Secure identity ready · pairing transport is next");
    emit stateChanged();
}

void IpcClient::setOffline(const QString &message)
{
    m_dropTimer.stop();
    const bool changed = m_serviceOnline || m_statusMessage != message || m_dropActive;
    m_serviceOnline = false;
    m_dropActive = false;
    m_pairingReady = false;
    m_transferReady = false;
    m_statusMessage = message;
    if (changed)
        emit stateChanged();
}
