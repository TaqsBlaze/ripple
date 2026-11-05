#pragma once

#include <QByteArray>
#include <QLocalSocket>
#include <QObject>
#include <QTimer>
#include <QJsonArray>
#include <QUrl>

class IpcClient final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool serviceOnline READ serviceOnline NOTIFY stateChanged)
    Q_PROPERTY(QString deviceName READ deviceName NOTIFY stateChanged)
    Q_PROPERTY(QString fingerprint READ fingerprint NOTIFY stateChanged)
    Q_PROPERTY(QString serviceVersion READ serviceVersion NOTIFY stateChanged)
    Q_PROPERTY(bool pairingReady READ pairingReady NOTIFY stateChanged)
    Q_PROPERTY(bool transferReady READ transferReady NOTIFY stateChanged)
    Q_PROPERTY(QString statusMessage READ statusMessage NOTIFY stateChanged)
    Q_PROPERTY(QString invitation READ invitation NOTIFY stateChanged)
    Q_PROPERTY(QString invitationQr READ invitationQr NOTIFY stateChanged)
    Q_PROPERTY(QJsonArray pendingPairs READ pendingPairs NOTIFY stateChanged)
    Q_PROPERTY(QJsonArray trustedPeers READ trustedPeers NOTIFY stateChanged)
    Q_PROPERTY(QJsonArray transfers READ transfers NOTIFY stateChanged)
    Q_PROPERTY(bool autoAccept READ autoAccept NOTIFY stateChanged)
    Q_PROPERTY(bool dropActive READ dropActive NOTIFY stateChanged)
    Q_PROPERTY(QString receiveDir READ receiveDir NOTIFY stateChanged)
    Q_PROPERTY(QString folderMessage READ folderMessage NOTIFY stateChanged)
    Q_PROPERTY(QString sendMessage READ sendMessage NOTIFY stateChanged)

public:
    explicit IpcClient(QObject *parent = nullptr);

    bool serviceOnline() const { return m_serviceOnline; }
    QString deviceName() const { return m_deviceName; }
    QString fingerprint() const { return m_fingerprint; }
    QString serviceVersion() const { return m_serviceVersion; }
    bool pairingReady() const { return m_pairingReady; }
    bool transferReady() const { return m_transferReady; }
    QString statusMessage() const { return m_statusMessage; }
    QString invitation() const { return m_invitation; }
    QString invitationQr() const { return m_invitationQr; }
    QJsonArray pendingPairs() const { return m_pendingPairs; }
    QJsonArray trustedPeers() const { return m_trustedPeers; }
    QJsonArray transfers() const { return m_transfers; }
    bool autoAccept() const { return m_autoAccept; }
    bool dropActive() const { return m_dropActive; }
    QString receiveDir() const { return m_receiveDir; }
    QString folderMessage() const { return m_folderMessage; }
    QString sendMessage() const { return m_sendMessage; }

    Q_INVOKABLE void refresh();
    Q_INVOKABLE void setDesktopDropActive(bool active);
    Q_INVOKABLE void createInvitation();
    Q_INVOKABLE void decidePairing(const QString &id, bool approve);
    Q_INVOKABLE void decideTransfer(const QString &id, bool approve);
    Q_INVOKABLE void setReceiveDir(const QUrl &url);
    Q_INVOKABLE void setAutoAccept(bool enabled);
    Q_INVOKABLE void setDeviceName(const QString &name);
    Q_INVOKABLE bool openReceivedFile(const QString &path);
    Q_INVOKABLE void deleteReceivedFile(const QString &id);
    Q_INVOKABLE void offerLocalFile(const QString &peerId, const QUrl &url);

signals:
    void stateChanged();

private slots:
    void connected();
    void disconnected();
    void readAvailable();
    void socketError(QLocalSocket::LocalSocketError error);

private:
    void requestStatus();
    void handleFrame(const QByteArray &frame);
    void setOffline(const QString &message);

    QLocalSocket m_socket;
    QTimer m_retryTimer;
    QTimer m_dropTimer;
    QByteArray m_buffer;
    bool m_serviceOnline = false;
    QString m_deviceName = QStringLiteral("This PC");
    QString m_fingerprint;
    QString m_serviceVersion;
    bool m_pairingReady = false;
    bool m_transferReady = false;
    QString m_statusMessage = QStringLiteral("Waiting for the Ripple service");
    QString m_invitation;
    QString m_invitationQr;
    QTimer m_invitationExpiry;
    QJsonArray m_pendingPairs;
    QJsonArray m_trustedPeers;
    QJsonArray m_transfers;
    bool m_autoAccept = false;
    bool m_dropActive = false;
    bool m_desktopDropActive = false;
    QString m_sendMessage;
    QString m_receiveDir;
    QString m_folderMessage;
};
