using System.Diagnostics;
using System.Drawing;
using System.Net.Sockets;
using System.Text;
using System.Windows.Forms;

namespace Sub2API.Tray;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        ApplicationConfiguration.Initialize();
        Application.Run(new TrayContext());
    }
}

internal sealed class TrayContext : ApplicationContext
{
    private readonly string rootDir;
    private readonly string startScript;
    private readonly string stopScript;
    private readonly string updateScript;
    private readonly NotifyIcon trayIcon;
    private readonly ToolStripMenuItem statusItem;
    private readonly System.Windows.Forms.Timer statusTimer;
    private bool busy;
    private bool openedOnStartup;

    public TrayContext()
    {
        rootDir = Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, ".."));
        startScript = Path.Combine(rootDir, "start.ps1");
        stopScript = Path.Combine(rootDir, "stop.ps1");
        updateScript = Path.Combine(rootDir, "update.ps1");

        statusItem = new ToolStripMenuItem("Status: checking") { Enabled = false };

        var menu = new ContextMenuStrip();
        menu.Items.Add(statusItem);
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Open dashboard", null, (_, _) => OpenUrl("http://localhost:8080"));
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Start / restart", null, async (_, _) => await StartServiceAsync());
        menu.Items.Add("Stop service", null, async (_, _) => await StopServiceAsync());
        menu.Items.Add("Update to latest", null, async (_, _) => await UpdateServiceAsync());
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Open logs folder", null, (_, _) => OpenPath(Path.Combine(rootDir, "runtime", "logs")));
        menu.Items.Add("Exit tray", null, (_, _) => ExitTray());

        trayIcon = new NotifyIcon
        {
            Icon = BuildIcon(Color.FromArgb(24, 119, 242)),
            Text = "Sub2API",
            ContextMenuStrip = menu,
            Visible = true
        };
        trayIcon.DoubleClick += (_, _) => OpenUrl("http://localhost:8080");

        statusTimer = new System.Windows.Forms.Timer { Interval = 5000 };
        statusTimer.Tick += async (_, _) => await RefreshStatusAsync(false);
        statusTimer.Start();

        _ = StartServiceAsync();
    }

    private async Task StartServiceAsync()
    {
        if (busy) return;
        busy = true;
        SetStatus("Status: starting", Color.DarkOrange);

        try
        {
            await RunPowerShellAsync(startScript);
            await RefreshStatusAsync(true);
            await OpenDashboardAfterStartupAsync();
        }
        catch (Exception ex)
        {
            SetStatus("Status: start failed", Color.Firebrick);
            ShowTip("Sub2API start failed", ex.Message, ToolTipIcon.Error);
        }
        finally
        {
            busy = false;
        }
    }

    private async Task StopServiceAsync()
    {
        if (busy) return;
        busy = true;
        SetStatus("Status: stopping", Color.DarkOrange);

        try
        {
            await RunPowerShellAsync(stopScript);
            await RefreshStatusAsync(true);
        }
        catch (Exception ex)
        {
            SetStatus("Status: stop failed", Color.Firebrick);
            ShowTip("Sub2API stop failed", ex.Message, ToolTipIcon.Error);
        }
        finally
        {
            busy = false;
        }
    }

    private async Task UpdateServiceAsync()
    {
        if (busy) return;
        busy = true;
        SetStatus("Status: updating", Color.DarkOrange);

        try
        {
            await RunPowerShellAsync(updateScript);
            await RefreshStatusAsync(true);
            await OpenDashboardAfterStartupAsync();
            ShowTip("Sub2API updated", "Updated to the latest GitHub release.", ToolTipIcon.Info);
        }
        catch (Exception ex)
        {
            SetStatus("Status: update failed", Color.Firebrick);
            ShowTip("Sub2API update failed", ex.Message, ToolTipIcon.Error);
        }
        finally
        {
            busy = false;
        }
    }

    private async Task OpenDashboardAfterStartupAsync()
    {
        if (openedOnStartup)
        {
            return;
        }

        if (!await IsPortOpenAsync("127.0.0.1", 8080))
        {
            return;
        }

        openedOnStartup = true;
        OpenUrl("http://localhost:8080");
    }

    private async Task RefreshStatusAsync(bool notify)
    {
        var running = await IsPortOpenAsync("127.0.0.1", 8080);
        if (running)
        {
            SetStatus("Status: running", Color.SeaGreen);
            if (notify) ShowTip("Sub2API is running", "Dashboard: http://localhost:8080", ToolTipIcon.Info);
        }
        else
        {
            SetStatus("Status: stopped", Color.Firebrick);
            if (notify) ShowTip("Sub2API is stopped", "Right-click the tray icon to start.", ToolTipIcon.Warning);
        }
    }

    private void SetStatus(string text, Color color)
    {
        statusItem.Text = text;
        trayIcon.Icon = BuildIcon(color);
        trayIcon.Text = text.Length > 63 ? text[..63] : text;
    }

    private static async Task<bool> IsPortOpenAsync(string host, int port)
    {
        try
        {
            using var client = new TcpClient();
            var connectTask = client.ConnectAsync(host, port);
            var finished = await Task.WhenAny(connectTask, Task.Delay(1000));
            return finished == connectTask && client.Connected;
        }
        catch
        {
            return false;
        }
    }

    private async Task RunPowerShellAsync(string scriptPath)
    {
        if (!File.Exists(scriptPath))
        {
            throw new FileNotFoundException($"Script not found: {scriptPath}");
        }

        var psi = new ProcessStartInfo
        {
            FileName = "powershell.exe",
            Arguments = $"-NoProfile -ExecutionPolicy Bypass -File \"{scriptPath}\"",
            WorkingDirectory = rootDir,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            StandardOutputEncoding = Encoding.UTF8,
            StandardErrorEncoding = Encoding.UTF8
        };

        using var process = Process.Start(psi) ?? throw new InvalidOperationException("Failed to start PowerShell.");
        var outputTask = process.StandardOutput.ReadToEndAsync();
        var errorTask = process.StandardError.ReadToEndAsync();
        await process.WaitForExitAsync();

        var output = await outputTask;
        var error = await errorTask;
        if (process.ExitCode != 0)
        {
            var detail = string.IsNullOrWhiteSpace(error) ? output : error;
            throw new InvalidOperationException(detail.Trim());
        }
    }

    private static void OpenUrl(string url)
    {
        Process.Start(new ProcessStartInfo { FileName = url, UseShellExecute = true });
    }

    private static void OpenPath(string path)
    {
        Directory.CreateDirectory(path);
        Process.Start(new ProcessStartInfo { FileName = path, UseShellExecute = true });
    }

    private void ShowTip(string title, string message, ToolTipIcon icon)
    {
        trayIcon.BalloonTipTitle = title;
        trayIcon.BalloonTipText = message;
        trayIcon.BalloonTipIcon = icon;
        trayIcon.ShowBalloonTip(3000);
    }

    private void ExitTray()
    {
        statusTimer.Stop();
        trayIcon.Visible = false;
        trayIcon.Dispose();
        ExitThread();
    }

    private static Icon BuildIcon(Color color)
    {
        using var bitmap = new Bitmap(32, 32);
        using var graphics = Graphics.FromImage(bitmap);
        graphics.Clear(Color.Transparent);
        graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.AntiAlias;

        using var brush = new SolidBrush(color);
        using var whiteBrush = new SolidBrush(Color.White);
        using var font = new Font("Segoe UI", 15, FontStyle.Bold, GraphicsUnit.Pixel);

        graphics.FillEllipse(brush, 2, 2, 28, 28);
        graphics.DrawString("S", font, whiteBrush, 9, 6);

        return Icon.FromHandle(bitmap.GetHicon());
    }
}
