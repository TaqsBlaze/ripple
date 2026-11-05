#include "ipcclient.h"

#include <QCoreApplication>
#include <QDir>
#include <QGuiApplication>
#include <QLocalSocket>
#include <QProcess>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QStandardPaths>
#include <QTimer>
#include <sys/prctl.h>
#include <signal.h>
#include <unistd.h>

int main(int argc, char *argv[])
{
    QGuiApplication application(argc, argv);
    application.setApplicationName(QStringLiteral("Ripple"));
    application.setOrganizationName(QStringLiteral("Ripple"));
    application.setApplicationVersion(QStringLiteral("0.1.0"));
    QQuickStyle::setStyle(QStringLiteral("Basic"));

    // Reuse an independently started service, but only manage a child we own.
    QLocalSocket probe;
    const QString socketPath = QDir(QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation))
                                   .filePath(QStringLiteral("ripple/ripple.sock"));
    probe.connectToServer(socketPath);
    const bool serviceAlreadyRunning = probe.waitForConnected(200);
    probe.abort();

    QProcess service;
    service.setProcessChannelMode(QProcess::ForwardedChannels);
    service.setProgram(QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("rippled")));
    // Also terminate our child if the desktop is killed without normal Qt cleanup.
    service.setChildProcessModifier([] {
        const pid_t parent = getppid();
        if (prctl(PR_SET_PDEATHSIG, SIGTERM) == -1 || getppid() != parent)
            _exit(1);
    });
    if (!serviceAlreadyRunning)
        service.start();

    IpcClient client;
    QObject::connect(&service, &QProcess::started, &client, &IpcClient::refresh);
    QObject::connect(&application, &QCoreApplication::aboutToQuit, &application, [&service] {
        if (service.state() == QProcess::NotRunning)
            return;
        service.terminate();
        if (!service.waitForFinished(5000)) {
            service.kill();
            service.waitForFinished(1000);
        }
    });
    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty(QStringLiteral("rippleService"), &client);
    QObject::connect(&engine, &QQmlApplicationEngine::objectCreationFailed,
                     &application, [] { QCoreApplication::exit(1); }, Qt::QueuedConnection);
    engine.load(QUrl(QStringLiteral("qrc:/qml/Main.qml")));
    return application.exec();
}
