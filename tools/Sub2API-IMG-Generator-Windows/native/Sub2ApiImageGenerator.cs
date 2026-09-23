using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Diagnostics;

namespace Sub2ApiImageGenerator
{
    internal static class Program
    {
        [STAThread]
        private static void Main()
        {
            ServicePointManager.SecurityProtocol = SecurityProtocolType.Tls12;
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            Application.Run(new MainForm());
        }
    }

    public sealed class MainForm : Form
    {
        private readonly TextBox apiKeyBox = new TextBox();
        private readonly TextBox baseUrlBox = new TextBox();
        private readonly TextBox modelBox = new TextBox();
        private readonly ComboBox qualityBox = new ComboBox();
        private readonly ComboBox sizeBox = new ComboBox();
        private readonly RichTextBox promptBox = new RichTextBox();
        private readonly PictureBox contextPreview = new PictureBox();
        private readonly Button browseContextButton = new Button();
        private readonly Button clearContextButton = new Button();
        private readonly Button generateButton = new Button();
        private readonly Button cancelButton = new Button();
        private readonly Label statusLabel = new Label();
        private readonly FlowLayoutPanel gallery = new FlowLayoutPanel();
        private HttpWebRequest currentRequest;
        private bool? promptIsRtl;
        private string contextImagePath = "";

        public MainForm()
        {
            Text = "Sub2API Image Generator";
            StartPosition = FormStartPosition.CenterScreen;
            WindowState = FormWindowState.Maximized;
            MinimumSize = new Size(980, 680);
            Size = new Size(1120, 760);
            BackColor = Color.FromArgb(18, 19, 21);
            ForeColor = Color.FromArgb(244, 241, 234);
            Font = new Font("Segoe UI", 9F, FontStyle.Regular, GraphicsUnit.Point);

            BuildLayout();
        }

        private void BuildLayout()
        {
            var root = new TableLayoutPanel
            {
                Dock = DockStyle.Fill,
                ColumnCount = 2,
                RowCount = 1,
                Padding = new Padding(18),
                BackColor = BackColor
            };
            root.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 380));
            root.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));

            var left = new Panel { Dock = DockStyle.Fill, Padding = new Padding(18), BackColor = Color.FromArgb(24, 26, 28) };
            var right = new Panel { Dock = DockStyle.Fill, Padding = new Padding(18), BackColor = Color.FromArgb(24, 26, 28) };
            root.Controls.Add(left, 0, 0);
            root.Controls.Add(right, 1, 0);
            Controls.Add(root);

            var leftLayout = new TableLayoutPanel
            {
                Dock = DockStyle.Fill,
                ColumnCount = 1,
                RowCount = 2,
                BackColor = left.BackColor
            };
            leftLayout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            leftLayout.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
            left.Controls.Add(leftLayout);

            var title = new Label
            {
                Text = "Sub2API Image Generator",
                Dock = DockStyle.Fill,
                AutoSize = true,
                MaximumSize = new Size(330, 0),
                Margin = new Padding(0, 0, 0, 24),
                Font = new Font("Segoe UI", 17F, FontStyle.Bold, GraphicsUnit.Point),
                ForeColor = Color.FromArgb(244, 241, 234)
            };
            leftLayout.Controls.Add(title, 0, 0);

            var form = new TableLayoutPanel
            {
                Dock = DockStyle.Fill,
                ColumnCount = 1,
                RowCount = 16,
                BackColor = left.BackColor
            };
            for (var i = 0; i < 16; i++) form.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            leftLayout.Controls.Add(form, 0, 1);

            apiKeyBox.UseSystemPasswordChar = true;
            apiKeyBox.PlaceholderTextCompat("sk-...");
            apiKeyBox.Text = LoadSavedApiKey();
            baseUrlBox.PlaceholderTextCompat("https://your-sub2api-host/v1");
            baseUrlBox.Text = LoadSavedBaseUrl();
            modelBox.Text = "gpt-image-2";
            qualityBox.DropDownStyle = ComboBoxStyle.DropDownList;
            qualityBox.Items.AddRange(new object[] { "low", "medium", "high" });
            qualityBox.SelectedIndex = 0;
            sizeBox.DropDownStyle = ComboBoxStyle.DropDownList;
            sizeBox.Items.AddRange(new object[] { "1K", "2K", "4K" });
            sizeBox.SelectedIndex = 0;
            promptBox.Multiline = true;
            promptBox.Height = 120;
            promptBox.ScrollBars = RichTextBoxScrollBars.Vertical;
            promptBox.BorderStyle = BorderStyle.FixedSingle;
            promptBox.DetectUrls = false;
            promptBox.TextChanged += delegate { UpdatePromptDirection(); };
            UpdatePromptDirection();

            AddField(form, "Sub2API Base URL", baseUrlBox);
            AddField(form, "OpenAI-compatible API Key", apiKeyBox);
            AddField(form, "Model", modelBox);
            AddField(form, "Effort", qualityBox);
            AddField(form, "Output Size", sizeBox);
            AddField(form, "Prompt", promptBox);
            AddField(form, "Reference Image", BuildContextImagePanel());

            generateButton.Text = "Generate";
            generateButton.Height = 42;
            generateButton.Dock = DockStyle.Top;
            generateButton.BackColor = Color.FromArgb(240, 180, 90);
            generateButton.ForeColor = Color.FromArgb(21, 18, 13);
            generateButton.FlatStyle = FlatStyle.Flat;
            generateButton.FlatAppearance.BorderSize = 0;
            generateButton.Font = new Font("Segoe UI", 10F, FontStyle.Bold, GraphicsUnit.Point);
            generateButton.Click += async delegate { await GenerateAsync(); };
            form.Controls.Add(generateButton);

            cancelButton.Text = "Cancel Generation";
            cancelButton.Height = 38;
            cancelButton.Dock = DockStyle.Top;
            cancelButton.Enabled = false;
            cancelButton.BackColor = Color.FromArgb(74, 79, 84);
            cancelButton.ForeColor = Color.FromArgb(244, 241, 234);
            cancelButton.FlatStyle = FlatStyle.Flat;
            cancelButton.FlatAppearance.BorderSize = 0;
            cancelButton.Click += delegate { CancelGeneration(); };
            form.Controls.Add(cancelButton);

            statusLabel.Text = "Enter your Sub2API base URL, API key, and prompt.";
            statusLabel.Dock = DockStyle.Top;
            statusLabel.Height = 36;
            statusLabel.ForeColor = Color.FromArgb(189, 183, 173);
            right.Controls.Add(statusLabel);

            gallery.Dock = DockStyle.Fill;
            gallery.AutoScroll = true;
            gallery.WrapContents = true;
            gallery.BackColor = right.BackColor;
            gallery.Padding = new Padding(0, 46, 0, 0);
            right.Controls.Add(gallery);

            LoadHistory();
        }

        private static void AddField(TableLayoutPanel form, string labelText, Control control)
        {
            var label = new Label
            {
                Text = labelText,
                Dock = DockStyle.Top,
                Height = 20,
                ForeColor = Color.FromArgb(216, 210, 199),
                Font = new Font("Segoe UI", 9F, FontStyle.Bold, GraphicsUnit.Point)
            };

            control.Dock = DockStyle.Top;
            control.Margin = new Padding(0, 0, 0, 10);
            control.BackColor = Color.FromArgb(28, 30, 32);
            control.ForeColor = Color.FromArgb(244, 241, 234);

            form.Controls.Add(label);
            form.Controls.Add(control);
        }

        private Control BuildContextImagePanel()
        {
            var panel = new TableLayoutPanel
            {
                Dock = DockStyle.Top,
                ColumnCount = 2,
                RowCount = 2,
                Height = 170,
                BackColor = Color.FromArgb(28, 30, 32),
                Margin = new Padding(0, 0, 0, 10)
            };
            panel.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 50));
            panel.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 50));
            panel.RowStyles.Add(new RowStyle(SizeType.Absolute, 38));
            panel.RowStyles.Add(new RowStyle(SizeType.Percent, 100));

            browseContextButton.Text = "Browse";
            browseContextButton.Dock = DockStyle.Fill;
            browseContextButton.BackColor = Color.FromArgb(74, 79, 84);
            browseContextButton.ForeColor = Color.FromArgb(244, 241, 234);
            browseContextButton.FlatStyle = FlatStyle.Flat;
            browseContextButton.FlatAppearance.BorderSize = 0;
            browseContextButton.Click += delegate { BrowseContextImage(); };

            clearContextButton.Text = "Clear";
            clearContextButton.Dock = DockStyle.Fill;
            clearContextButton.Enabled = false;
            clearContextButton.BackColor = Color.FromArgb(52, 56, 60);
            clearContextButton.ForeColor = Color.FromArgb(244, 241, 234);
            clearContextButton.FlatStyle = FlatStyle.Flat;
            clearContextButton.FlatAppearance.BorderSize = 0;
            clearContextButton.Click += delegate { ClearContextImage(); };

            contextPreview.Dock = DockStyle.Fill;
            contextPreview.SizeMode = PictureBoxSizeMode.Zoom;
            contextPreview.BackColor = Color.FromArgb(13, 14, 16);

            panel.Controls.Add(browseContextButton, 0, 0);
            panel.Controls.Add(clearContextButton, 1, 0);
            panel.Controls.Add(contextPreview, 0, 1);
            panel.SetColumnSpan(contextPreview, 2);
            return panel;
        }

        private void UpdatePromptDirection()
        {
            var rtl = FirstWordUsesArabicScript(promptBox.Text);
            if (promptIsRtl.HasValue && promptIsRtl.Value == rtl) return;

            promptIsRtl = rtl;
            var start = promptBox.SelectionStart;
            var length = promptBox.SelectionLength;

            promptBox.RightToLeft = rtl ? RightToLeft.Yes : RightToLeft.No;
            ApplyPromptAlignment(rtl ? HorizontalAlignment.Right : HorizontalAlignment.Left);
            if (!IsHandleCreated)
            {
                RestorePromptSelection(start, length);
                return;
            }

            BeginInvoke(new Action(delegate
            {
                RestorePromptSelection(start, length);
            }));
        }

        private void ApplyPromptAlignment(HorizontalAlignment alignment)
        {
            promptBox.SelectAll();
            promptBox.SelectionAlignment = alignment;
        }

        private void RestorePromptSelection(int start, int length)
        {
            promptBox.SelectionStart = Math.Min(start, promptBox.TextLength);
            promptBox.SelectionLength = Math.Min(length, promptBox.TextLength - promptBox.SelectionStart);
        }

        private void BrowseContextImage()
        {
            using (var dialog = new OpenFileDialog())
            {
                dialog.Title = "Select reference image";
                dialog.Filter = "Image files|*.png;*.jpg;*.jpeg;*.webp;*.bmp|All files|*.*";
                dialog.Multiselect = false;
                if (dialog.ShowDialog(this) != DialogResult.OK) return;
                SetContextImage(dialog.FileName);
            }
        }

        private void SetContextImage(string path)
        {
            if (!File.Exists(path))
            {
                SetStatus("Reference image was not found.", true);
                return;
            }

            try
            {
                using (var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
                using (var image = Image.FromStream(fs))
                {
                    if (contextPreview.Image != null) contextPreview.Image.Dispose();
                    contextPreview.Image = new Bitmap(image);
                }
                contextImagePath = path;
                clearContextButton.Enabled = true;
                SetStatus("Reference image selected: " + Path.GetFileName(path), false);
            }
            catch (Exception ex)
            {
                SetStatus("Could not load reference image: " + ex.Message, true);
            }
        }

        private void ClearContextImage()
        {
            contextImagePath = "";
            clearContextButton.Enabled = false;
            if (contextPreview.Image != null)
            {
                contextPreview.Image.Dispose();
                contextPreview.Image = null;
            }
            SetStatus("Reference image cleared.", false);
        }

        private static bool FirstWordUsesArabicScript(string text)
        {
            if (string.IsNullOrWhiteSpace(text)) return false;

            var i = 0;
            while (i < text.Length && char.IsWhiteSpace(text[i])) i++;
            while (i < text.Length && IsWordBoundaryPunctuation(text[i])) i++;

            for (; i < text.Length; i++)
            {
                var ch = text[i];
                if (char.IsWhiteSpace(ch)) break;
                if (IsArabicScriptChar(ch)) return true;
                if (char.IsLetterOrDigit(ch)) return false;
            }

            return false;
        }

        private static bool IsWordBoundaryPunctuation(char ch)
        {
            return char.IsPunctuation(ch) || char.IsSymbol(ch);
        }

        private static bool IsArabicScriptChar(char ch)
        {
            var code = (int)ch;
            return (code >= 0x0600 && code <= 0x06FF) ||
                   (code >= 0x0750 && code <= 0x077F) ||
                   (code >= 0x08A0 && code <= 0x08FF) ||
                   (code >= 0xFB50 && code <= 0xFDFF) ||
                   (code >= 0xFE70 && code <= 0xFEFF);
        }

        private async Task GenerateAsync()
        {
            var apiKey = apiKeyBox.Text.Trim();
            var baseUrl = NormalizeBaseUrl(baseUrlBox.Text);
            var model = modelBox.Text.Trim();
            var quality = Convert.ToString(qualityBox.SelectedItem);
            var size = ResolveSizeValue(Convert.ToString(sizeBox.SelectedItem));
            var prompt = promptBox.Text.Trim();

            if (baseUrl.Length == 0) { SetStatus("Sub2API base URL is required.", true); return; }
            if (apiKey.Length == 0) { SetStatus("API key is required.", true); return; }
            if (quality.Length == 0) { SetStatus("Quality is required.", true); return; }
            if (size.Length == 0) { SetStatus("Size is required.", true); return; }
            if (prompt.Length == 0) { SetStatus("Prompt is required.", true); return; }

            SaveApiKey(apiKey);
            SaveBaseUrl(baseUrl);
            generateButton.Enabled = false;
            cancelButton.Enabled = true;
            SetStatus("Generating image...", false);

            try
            {
                var imageDataUrl = BuildContextImageDataUrl();
                var endpoint = imageDataUrl.Length == 0 ? "/images/generations" : "/images/edits";
                var json = BuildRequestJson(model, prompt, quality, size, imageDataUrl);
                var response = await PostJsonAsync(baseUrl + endpoint, apiKey, json);
                var images = ExtractImages(response);
                if (images.Count == 0)
                {
                    SetStatus("The API returned no image data.", true);
                    return;
                }

                var savedCount = 0;
                foreach (var image in images)
                {
                    var savedPath = await SaveGeneratedImageAsync(image);
                    AddImageFile(savedPath, true);
                    savedCount++;
                }
                SetStatus(savedCount + " image" + (savedCount == 1 ? "" : "s") + " generated and saved.", false);
            }
            catch (WebException ex)
            {
                if (ex.Status == WebExceptionStatus.RequestCanceled)
                {
                    SetStatus("Generation cancelled.", true);
                }
                else
                {
                    SetStatus(ex.Message, true);
                }
            }
            catch (Exception ex)
            {
                SetStatus(ex.Message, true);
            }
            finally
            {
                currentRequest = null;
                generateButton.Enabled = true;
                cancelButton.Enabled = false;
            }
        }

        private static string ResolveSizeValue(string selectedSize)
        {
            switch ((selectedSize ?? "").Trim().ToUpperInvariant())
            {
                case "1K": return "1024x1024";
                case "2K": return "2048x2048";
                case "4K": return "3840x2160";
                default: return "1024x1024";
            }
        }

        private static string NormalizeBaseUrl(string value)
        {
            var baseUrl = (value ?? "").Trim();
            if (baseUrl.Length == 0) return "";

            if (!baseUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase) &&
                !baseUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            {
                baseUrl = LooksLikeLocalHost(baseUrl) ? "http://" + baseUrl : "https://" + baseUrl;
            }

            baseUrl = baseUrl.TrimEnd('/');
            if (!baseUrl.EndsWith("/v1", StringComparison.OrdinalIgnoreCase))
            {
                baseUrl += "/v1";
            }

            return baseUrl;
        }

        private static bool LooksLikeLocalHost(string value)
        {
            return Regex.IsMatch(value, "^(localhost|127\\.|10\\.|192\\.168\\.|172\\.(1[6-9]|2[0-9]|3[01])\\.|[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+)(:|/|$)", RegexOptions.IgnoreCase);
        }

        private string BuildContextImageDataUrl()
        {
            if (contextImagePath.Length == 0) return "";
            if (!File.Exists(contextImagePath)) throw new FileNotFoundException("Reference image was not found.", contextImagePath);
            var bytes = File.ReadAllBytes(contextImagePath);
            return "data:" + GetImageMimeType(contextImagePath) + ";base64," + Convert.ToBase64String(bytes);
        }

        private static string GetImageMimeType(string path)
        {
            switch (Path.GetExtension(path).ToLowerInvariant())
            {
                case ".jpg":
                case ".jpeg": return "image/jpeg";
                case ".webp": return "image/webp";
                case ".bmp": return "image/bmp";
                default: return "image/png";
            }
        }

        private static string BuildRequestJson(string model, string prompt, string quality, string size, string imageDataUrl)
        {
            var sb = new StringBuilder();
            sb.Append("{");
            AddJsonPair(sb, "model", model, false);
            AddJsonPair(sb, "prompt", prompt, true);
            sb.Append(",\"n\":1");
            AddJsonPair(sb, "quality", quality, true);
            AddJsonPair(sb, "size", size, true);
            if (imageDataUrl.Length > 0)
            {
                sb.Append(",\"images\":[{");
                AddJsonPair(sb, "image_url", imageDataUrl, false);
                sb.Append("}]");
            }
            sb.Append("}");
            return sb.ToString();
        }

        private static void AddJsonPair(StringBuilder sb, string key, string value, bool prefixComma)
        {
            if (prefixComma) sb.Append(",");
            sb.Append("\"").Append(EscapeJson(key)).Append("\":\"").Append(EscapeJson(value)).Append("\"");
        }

        private async Task<string> PostJsonAsync(string url, string apiKey, string json)
        {
            var request = (HttpWebRequest)WebRequest.Create(url);
            currentRequest = request;
            request.Method = "POST";
            request.ContentType = "application/json";
            request.Headers["Authorization"] = "Bearer " + apiKey;
            request.Timeout = 600000;

            var data = Encoding.UTF8.GetBytes(json);
            request.ContentLength = data.Length;
            using (var stream = await request.GetRequestStreamAsync())
            {
                await stream.WriteAsync(data, 0, data.Length);
            }

            try
            {
                using (var response = (HttpWebResponse)await request.GetResponseAsync())
                using (var reader = new StreamReader(response.GetResponseStream()))
                {
                    return await reader.ReadToEndAsync();
                }
            }
            catch (WebException ex)
            {
                if (ex.Response == null) throw;
                using (var reader = new StreamReader(ex.Response.GetResponseStream()))
                {
                    var body = reader.ReadToEnd();
                    throw new Exception(ExtractError(body));
                }
            }
        }

        private static List<string> ExtractImages(string json)
        {
            var images = new List<string>();
            foreach (Match match in Regex.Matches(json, "\"url\"\\s*:\\s*\"([^\"]+)\""))
            {
                images.Add(UnescapeJson(match.Groups[1].Value));
            }
            foreach (Match match in Regex.Matches(json, "\"b64_json\"\\s*:\\s*\"([^\"]+)\""))
            {
                images.Add("data:image/png;base64," + match.Groups[1].Value);
            }
            return images;
        }

        private static string ExtractError(string json)
        {
            var message = Regex.Match(json, "\"message\"\\s*:\\s*\"([^\"]+)\"");
            if (message.Success) return UnescapeJson(message.Groups[1].Value);

            var error = Regex.Match(json, "\"error\"\\s*:\\s*\"([^\"]+)\"");
            if (error.Success) return UnescapeJson(error.Groups[1].Value);

            return json.Length > 500 ? json.Substring(0, 500) : json;
        }

        private void CancelGeneration()
        {
            try
            {
                if (currentRequest != null) currentRequest.Abort();
            }
            catch
            {
            }
        }

        private async Task<string> SaveGeneratedImageAsync(string source)
        {
            Directory.CreateDirectory(ImagesDirectory);
            var path = Path.Combine(ImagesDirectory, "image-" + DateTime.Now.ToString("yyyyMMdd-HHmmss-fff") + "-" + Guid.NewGuid().ToString("N").Substring(0, 8) + ".png");

            if (source.StartsWith("data:image", StringComparison.OrdinalIgnoreCase))
            {
                var comma = source.IndexOf(',');
                var bytes = Convert.FromBase64String(source.Substring(comma + 1));
                File.WriteAllBytes(path, bytes);
                return path;
            }

            using (var client = new WebClient())
            {
                var bytes = await client.DownloadDataTaskAsync(source);
                File.WriteAllBytes(path, bytes);
            }
            return path;
        }

        private void LoadHistory()
        {
            Directory.CreateDirectory(ImagesDirectory);
            var files = new List<string>();
            files.AddRange(Directory.GetFiles(ImagesDirectory, "*.png"));
            files.AddRange(Directory.GetFiles(ImagesDirectory, "*.jpg"));
            files.AddRange(Directory.GetFiles(ImagesDirectory, "*.jpeg"));
            files.Sort(StringComparer.OrdinalIgnoreCase);
            files.Reverse();

            foreach (var file in files)
            {
                AddImageFile(file, false);
            }

            if (files.Count > 0)
            {
                SetStatus(files.Count + " saved image" + (files.Count == 1 ? "" : "s") + " loaded from history.", false);
            }
        }

        private void AddImageFile(string path, bool addToTop)
        {
            var panel = new Panel { Width = 260, Height = 310, Margin = new Padding(0, 0, 16, 16), BackColor = Color.FromArgb(18, 19, 21) };
            var picture = new PictureBox { Width = 260, Height = 260, Dock = DockStyle.Top, SizeMode = PictureBoxSizeMode.Zoom, BackColor = Color.FromArgb(13, 14, 16) };
            var button = new Button { Text = "Open image", Dock = DockStyle.Bottom, Height = 38, Tag = path };
            button.Click += delegate { OpenImage((string)button.Tag); };

            using (var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
            using (var image = Image.FromStream(fs))
            {
                picture.Image = new Bitmap(image);
            }

            panel.Controls.Add(button);
            panel.Controls.Add(picture);
            if (addToTop && gallery.Controls.Count > 0)
            {
                gallery.Controls.Add(panel);
                gallery.Controls.SetChildIndex(panel, 0);
            }
            else
            {
                gallery.Controls.Add(panel);
            }
        }

        private static void OpenImage(string source)
        {
            Process.Start(source);
        }

        private void SetStatus(string text, bool error)
        {
            statusLabel.Text = text;
            statusLabel.ForeColor = error ? Color.FromArgb(255, 176, 163) : Color.FromArgb(189, 183, 173);
        }

        private static string LoadSavedApiKey()
        {
            try
            {
                return File.Exists(ConfigPath) ? File.ReadAllText(ConfigPath).Trim() : "";
            }
            catch
            {
                return "";
            }
        }

        private static void SaveApiKey(string apiKey)
        {
            try
            {
                File.WriteAllText(ConfigPath, apiKey);
            }
            catch
            {
            }
        }

        private static string LoadSavedBaseUrl()
        {
            try
            {
                return File.Exists(BaseUrlConfigPath) ? File.ReadAllText(BaseUrlConfigPath).Trim() : "";
            }
            catch
            {
                return "";
            }
        }

        private static void SaveBaseUrl(string baseUrl)
        {
            try
            {
                File.WriteAllText(BaseUrlConfigPath, baseUrl);
            }
            catch
            {
            }
        }

        private static string AppDirectory
        {
            get { return AppDomain.CurrentDomain.BaseDirectory; }
        }

        private static string ImagesDirectory
        {
            get { return Path.Combine(AppDirectory, "GeneratedImages"); }
        }

        private static string ConfigPath
        {
            get { return Path.Combine(AppDirectory, "api-key.txt"); }
        }

        private static string BaseUrlConfigPath
        {
            get { return Path.Combine(AppDirectory, "base-url.txt"); }
        }

        private static string EscapeJson(string value)
        {
            return value.Replace("\\", "\\\\").Replace("\"", "\\\"").Replace("\r", "\\r").Replace("\n", "\\n");
        }

        private static string UnescapeJson(string value)
        {
            return Regex.Unescape(value);
        }
    }

    internal static class TextBoxExtensions
    {
        public static void PlaceholderTextCompat(this TextBox box, string text)
        {
            if (Environment.OSVersion.Version.Major >= 10)
            {
                NativeMethods.SendMessage(box.Handle, 0x1501, (IntPtr)1, text);
            }
        }
    }

    internal static class NativeMethods
    {
        [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode)]
        public static extern IntPtr SendMessage(IntPtr hWnd, int msg, IntPtr wParam, string lParam);
    }
}
