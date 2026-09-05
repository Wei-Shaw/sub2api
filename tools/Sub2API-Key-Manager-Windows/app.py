import json
import os
import re
import shutil
import subprocess
import sys
import threading
import urllib.error
import urllib.parse
import urllib.request
import webbrowser
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from tkinter import Canvas, StringVar, Tk, messagebox, ttk


APP_NAME = "Sub2API Key Manager"
ENV_FILE_NAME = "sub2api_key_manager.env"
CODEX_DOWNLOAD_URL = "https://chatgpt.com/codex/"
OPENCODE_DOWNLOAD_URL = "https://opencode.ai/download"

BG = "#0f172a"
PANEL = "#111c33"
TEXT = "#e5edf7"
MUTED = "#8ea4c2"
BORDER = "#263854"
GREEN = "#2dd4bf"
BLUE = "#60a5fa"
RED = "#fb7185"
AMBER = "#fbbf24"


def app_dir() -> Path:
    if getattr(sys, "frozen", False):
        return Path(sys.executable).resolve().parent
    return Path(__file__).resolve().parent


def resource_path(name: str) -> Path:
    if getattr(sys, "frozen", False) and hasattr(sys, "_MEIPASS"):
        return Path(sys._MEIPASS) / name
    return app_dir() / name


def load_env_config() -> dict[str, str]:
    values = {"endpoint": "", "status_token": ""}
    env_path = app_dir() / ENV_FILE_NAME
    if not env_path.exists():
        return values
    for raw in env_path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip().upper()
        value = value.strip().strip('"').strip("'")
        if key == "SUB2API_ENDPOINT":
            values["endpoint"] = value
        elif key == "STATUS_API_TOKEN":
            values["status_token"] = value
    return values


def normalize_endpoint(value: str) -> str:
    value = value.strip().rstrip("/")
    if value and not re.match(r"^https?://", value, re.IGNORECASE):
        value = "http://" + value
    return value


def money(value: object) -> float:
    try:
        return float(value or 0)
    except (TypeError, ValueError):
        return 0.0


def format_local_datetime(value: str | None) -> str | None:
    if not value:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        return parsed.astimezone().strftime("%Y/%m/%d %H:%M:%S")
    except ValueError:
        return value


def preview_key(value: str) -> str:
    value = value.strip()
    if len(value) <= 16:
        return value
    return f"{value[:10]}...{value[-6:]}"


class AppPaths:
    home = Path.home()
    local_app_data = Path(os.environ.get("LOCALAPPDATA", home / "AppData" / "Local"))
    codex_dir = home / ".codex"
    codex_config = codex_dir / "config.toml"
    codex_auth = codex_dir / "auth.json"
    opencode_dir = home / ".config" / "opencode"
    opencode_config = opencode_dir / "opencode.jsonc"


def open_url(url: str):
    webbrowser.open(url, new=2)


def command_path(command: str) -> str | None:
    found = shutil.which(command)
    if found:
        return found
    try:
        result = subprocess.run(["where.exe", command], capture_output=True, text=True, timeout=2, check=False)
    except Exception:
        return None
    if result.returncode != 0:
        return None
    for line in result.stdout.splitlines():
        candidate = line.strip()
        if candidate:
            return candidate
    return None


def detect_codex() -> tuple[bool, str]:
    candidates = [
        AppPaths.codex_dir,
        AppPaths.local_app_data / "Programs" / "ChatGPT" / "ChatGPT.exe",
        AppPaths.local_app_data / "Microsoft" / "WindowsApps" / "ChatGPT.exe",
    ]
    for path in candidates:
        if path.exists():
            return True, str(path)
    found = command_path("codex")
    if found:
        return True, found
    return False, CODEX_DOWNLOAD_URL


def detect_opencode() -> tuple[bool, str]:
    candidates = [
        AppPaths.opencode_dir,
        AppPaths.local_app_data / "Programs" / "opencode" / "opencode.exe",
        AppPaths.local_app_data / "Microsoft" / "WindowsApps" / "opencode.exe",
    ]
    for path in candidates:
        if path.exists():
            return True, str(path)
    found = command_path("opencode")
    if found:
        return True, found
    return False, OPENCODE_DOWNLOAD_URL


def backup_if_exists(path: Path) -> Path | None:
    if not path.exists():
        return None
    backup = path.with_name(path.name + "." + datetime.now().strftime("%Y%m%d-%H%M%S") + ".bak")
    shutil.copy2(path, backup)
    return backup


def json_unescape(value: str) -> str:
    try:
        return json.loads('"' + value + '"')
    except Exception:
        return value


def find_json_string(text: str, key: str) -> str:
    match = re.search(r'"' + re.escape(key) + r'"\s*:\s*"((?:\\.|[^"\\])*)"', text)
    return json_unescape(match.group(1)) if match else ""


def find_provider_api_key(text: str, provider: str) -> str:
    pattern = r'"' + re.escape(provider) + r'"\s*:\s*\{[\s\S]*?"apiKey"\s*:\s*"((?:\\.|[^"\\])*)"'
    match = re.search(pattern, text, flags=re.IGNORECASE)
    return json_unescape(match.group(1)) if match else ""


def load_existing_keys() -> tuple[str, str]:
    openai_key = ""
    gemini_key = ""
    if AppPaths.codex_auth.exists():
        try:
            openai_key = find_json_string(AppPaths.codex_auth.read_text(encoding="utf-8"), "OPENAI_API_KEY")
        except Exception:
            pass
    if AppPaths.opencode_config.exists():
        try:
            text = AppPaths.opencode_config.read_text(encoding="utf-8")
            openai_key = find_provider_api_key(text, "openai") or openai_key
            gemini_key = find_provider_api_key(text, "google") or gemini_key
        except Exception:
            pass
    return openai_key, gemini_key


class TomlEditor:
    def __init__(self):
        self.order: list[str] = []
        self.sections: dict[str, list[str]] = {}

    @classmethod
    def parse(cls, text: str):
        editor = cls()
        editor.ensure_section("")
        current = ""
        for line in re.split(r"\r\n|\n|\r", text or ""):
            section = cls.parse_section_header(line)
            if section is not None:
                current = section
                editor.ensure_section(current)
            else:
                editor.sections[current].append(line)
        return editor

    @staticmethod
    def parse_section_header(line: str) -> str | None:
        trimmed = (line or "").strip()
        if trimmed.startswith("[[") or not trimmed.startswith("[") or not trimmed.endswith("]"):
            return None
        return trimmed[1:-1].strip()

    def ensure_section(self, section: str):
        if section not in self.sections:
            self.sections[section] = []
            self.order.append(section)

    def remove_keys(self, section: str, keys: list[str]):
        self.sections[section] = [
            line for line in self.sections[section]
            if not any(re.match(r"^\s*" + re.escape(key) + r"\s*=", line) for key in keys)
        ]

    def set_top_level(self, values: dict[str, str]):
        self.ensure_section("")
        self.remove_keys("", list(values.keys()))
        new_lines = [f"{key} = {value}" for key, value in values.items()]
        if self.sections[""] and self.sections[""][0]:
            new_lines.append("")
        new_lines.extend(self.sections[""])
        self.sections[""] = new_lines

    def set_section_values(self, section: str, values: dict[str, str]):
        self.ensure_section(section)
        self.remove_keys(section, list(values.keys()))
        if self.sections[section] and self.sections[section][-1]:
            self.sections[section].append("")
        self.sections[section].extend(f"{key} = {value}" for key, value in values.items())

    def render(self) -> str:
        output: list[str] = []
        for section in self.order:
            while self.sections[section] and not self.sections[section][-1]:
                self.sections[section].pop()
            if section:
                if output and output[-1]:
                    output.append("")
                output.append(f"[{section}]")
            output.extend(self.sections[section])
        while output and not output[-1]:
            output.pop()
        return "\n".join(output) + "\n"


def toml_string(value: str) -> str:
    return json.dumps(value)


def write_codex_config(base_url: str):
    AppPaths.codex_dir.mkdir(parents=True, exist_ok=True)
    existing = AppPaths.codex_config.read_text(encoding="utf-8") if AppPaths.codex_config.exists() else ""
    editor = TomlEditor.parse(existing)
    editor.set_top_level({
        "model_provider": toml_string("OpenAI"),
        "model": toml_string("gpt-5.5"),
        "review_model": toml_string("gpt-5.5"),
        "model_reasoning_effort": toml_string("xhigh"),
        "disable_response_storage": "true",
        "network_access": toml_string("enabled"),
        "windows_wsl_setup_acknowledged": "true",
    })
    editor.set_section_values("model_providers.OpenAI", {
        "name": toml_string("OpenAI"),
        "base_url": toml_string(base_url.rstrip("/")),
        "wire_api": toml_string("responses"),
        "supports_websockets": "true",
        "requires_openai_auth": "true",
    })
    editor.set_section_values("features", {"responses_websockets_v2": "true", "goals": "true"})
    AppPaths.codex_config.write_text(editor.render(), encoding="utf-8")


def write_codex_auth(openai_key: str):
    AppPaths.codex_dir.mkdir(parents=True, exist_ok=True)
    AppPaths.codex_auth.write_text(json.dumps({"OPENAI_API_KEY": openai_key}, indent=2) + "\n", encoding="utf-8")


def gemini_model_block(model_id: str, name: str) -> dict:
    return {
        "id": model_id,
        "name": name,
        "family": "gemini",
        "release_date": "2026-06-01",
        "attachment": True,
        "reasoning": True,
        "temperature": True,
        "tool_call": True,
        "limit": {"context": 1048576, "output": 32000},
    }


def write_opencode_config(openai_key: str, gemini_key: str, base_url: str):
    AppPaths.opencode_dir.mkdir(parents=True, exist_ok=True)
    clean_base = base_url.rstrip("/")
    config: dict[str, object] = {"$schema": "https://opencode.ai/config.json"}
    providers: dict[str, object] = {}
    if gemini_key:
        config["model"] = "google/gemini-3.5-flash"
        config["small_model"] = "google/gemini-3.5-flash"
        providers["google"] = {
            "options": {
                "apiKey": gemini_key,
                "baseURL": f"{clean_base}/antigravity/v1beta",
                "timeout": 600000,
                "chunkTimeout": 60000,
            },
            "models": {
                "gemini-3.5-flash-low": gemini_model_block("gemini-3.5-flash-low", "Gemini 3.5 Flash Low"),
                "gemini-3.5-flash-extra-low": gemini_model_block("gemini-3.5-flash-extra-low", "Gemini 3.5 Flash Extra Low"),
            },
        }
    if openai_key:
        providers["openai"] = {
            "options": {"apiKey": openai_key, "baseURL": f"{clean_base}/v1", "timeout": 600000, "chunkTimeout": 60000}
        }
    config["provider"] = providers
    AppPaths.opencode_config.write_text(json.dumps(config, indent=2) + "\n", encoding="utf-8")


class KeyStatusClient:
    def __init__(self, endpoint: str, token: str):
        self.endpoint = normalize_endpoint(endpoint)
        self.token = token.strip()

    def lookup(self, key: str) -> dict:
        if not self.endpoint:
            raise ValueError("Set the Sub2API endpoint first.")
        query = urllib.parse.urlencode({"key": key})
        headers = {"Accept": "application/json"}
        if self.token:
            headers["X-API-Key"] = self.token
        request = urllib.request.Request(f"{self.endpoint}/api-keys/status?{query}", headers=headers)
        with urllib.request.urlopen(request, timeout=12) as response:
            return json.loads(response.read().decode("utf-8"))


@dataclass
class KeyResult:
    category: str
    key: str
    valid: bool
    title: str
    status: str
    group: str = "-"
    expires_at: str | None = None
    charged: float = 0.0
    consumed: float = 0.0
    remaining: float = 0.0
    usable: bool = False


class RingChart(Canvas):
    def __init__(self, master, size=86, **kwargs):
        super().__init__(master, width=size, height=size, bg=PANEL, highlightthickness=0, **kwargs)
        self.size = size

    def draw(self, consumed: float, remaining: float, valid: bool):
        self.delete("all")
        pad = 9
        total = max(consumed + remaining, 0.01)
        extent = max(3, min(357, consumed / total * 360)) if valid else 360
        accent = GREEN if valid else RED
        self.create_oval(pad, pad, self.size - pad, self.size - pad, outline="#253654", width=9)
        self.create_arc(pad, pad, self.size - pad, self.size - pad, start=90, extent=-extent, outline=accent, width=9, style="arc")
        self.create_text(self.size / 2, self.size / 2, text="OK" if valid else "BAD", fill=TEXT, font=("Segoe UI", 11, "bold"))


class App(Tk):
    def __init__(self):
        super().__init__()
        self.title(APP_NAME)
        icon = resource_path("ai_key_market.ico")
        if icon.exists():
            self.iconbitmap(str(icon))
        self.geometry("1080x780")
        self.minsize(980, 700)
        self.state("zoomed")
        self.configure(bg=BG)
        self.config = load_env_config()
        endpoint = normalize_endpoint(self.config.get("endpoint", ""))
        self.config["endpoint"] = endpoint
        self.status = StringVar(value="Ready")
        self.config_status = StringVar(value=self.config_message())
        self.codex_status = StringVar(value="Codex: checking...")
        self.opencode_status = StringVar(value="OpenCode: checking...")
        self._style()
        self._build()
        self.refresh_app_status()
        self.after(300, self.load_existing_keys_and_validate)

    def _style(self):
        style = ttk.Style(self)
        style.theme_use("clam")
        style.configure("TFrame", background=BG)
        style.configure("Panel.TFrame", background=PANEL)
        style.configure("TLabel", background=BG, foreground=TEXT, font=("Segoe UI", 10))
        style.configure("Panel.TLabel", background=PANEL, foreground=TEXT, font=("Segoe UI", 10))
        style.configure("Title.TLabel", background=BG, foreground=TEXT, font=("Segoe UI", 22, "bold"))
        style.configure("Subtitle.TLabel", background=BG, foreground=MUTED, font=("Segoe UI", 10))
        style.configure("CardTitle.TLabel", background=PANEL, foreground=TEXT, font=("Segoe UI", 12, "bold"))
        style.configure("Status.TLabel", background=PANEL, foreground=MUTED, font=("Segoe UI", 9))
        style.configure("TButton", font=("Segoe UI", 10, "bold"), padding=(18, 10), borderwidth=0)
        style.map("TButton", background=[("active", BLUE), ("!disabled", "#2563eb")], foreground=[("!disabled", "white")])
        style.configure("Key.TEntry", fieldbackground="#0b1222", foreground=TEXT, bordercolor=BORDER, padding=(12, 12), font=("Cascadia Mono", 10))

    def _build(self):
        root = ttk.Frame(self, padding=22)
        root.pack(fill="both", expand=True)
        ttk.Label(root, text=APP_NAME, style="Title.TLabel").pack(anchor="w")

        config_panel = ttk.Frame(root, style="Panel.TFrame", padding=14)
        config_panel.pack(fill="x", pady=(18, 14))
        ttk.Label(config_panel, text="Service configuration", style="CardTitle.TLabel").pack(anchor="w")
        ttk.Label(config_panel, textvariable=self.config_status, style="Status.TLabel").pack(anchor="w", pady=(4, 0))

        keys = ttk.Frame(root)
        keys.pack(fill="x", pady=(0, 14))
        keys.columnconfigure(0, weight=1)
        keys.columnconfigure(1, weight=1)
        self.openai_key = self._input_panel(keys, "OpenAI / Codex", "Enter one OpenAI or Codex API key", 0)
        self.gemini_key = self._input_panel(keys, "Gemini", "Enter one Gemini API key", 1)

        controls = ttk.Frame(root)
        controls.pack(fill="x")
        self.validate_button = ttk.Button(controls, text="Validate Keys", command=self.validate_keys)
        self.validate_button.pack(side="left")
        ttk.Label(controls, textvariable=self.status, style="Subtitle.TLabel").pack(side="left", padx=(14, 0))

        apps = ttk.Frame(root)
        apps.pack(fill="x", pady=(14, 0))
        apps.columnconfigure(0, weight=1)
        apps.columnconfigure(1, weight=1)
        self.codex_button = self._app_panel(apps, "Codex", self.codex_status, self.configure_codex, self.download_codex, 0)
        self.opencode_button = self._app_panel(apps, "OpenCode", self.opencode_status, self.configure_opencode, self.download_opencode, 1)

        results = ttk.Frame(root, style="Panel.TFrame", padding=14)
        results.pack(fill="both", expand=True, pady=(16, 0))
        ttk.Label(results, text="Results", style="CardTitle.TLabel").pack(anchor="w", pady=(0, 10))
        self.results_frame = ttk.Frame(results, style="Panel.TFrame")
        self.results_frame.pack(fill="both", expand=True)
        self.results_frame.columnconfigure(0, weight=1)
        self.results_frame.columnconfigure(1, weight=1)
        self.empty_results()

    def _input_panel(self, parent, title: str, hint: str, column: int) -> ttk.Entry:
        panel = ttk.Frame(parent, style="Panel.TFrame", padding=14)
        panel.grid(row=0, column=column, sticky="ew", padx=(0, 10) if column == 0 else (10, 0))
        ttk.Label(panel, text=title, style="CardTitle.TLabel").pack(anchor="w")
        ttk.Label(panel, text=hint, style="Status.TLabel").pack(anchor="w", pady=(3, 10))
        entry = ttk.Entry(panel, style="Key.TEntry")
        entry.pack(fill="x")
        return entry

    def _app_panel(self, parent, title: str, status_var: StringVar, configure, download, column: int):
        panel = ttk.Frame(parent, style="Panel.TFrame", padding=14)
        panel.grid(row=0, column=column, sticky="ew", padx=(0, 10) if column == 0 else (10, 0))
        ttk.Label(panel, text=title, style="CardTitle.TLabel").pack(anchor="w")
        ttk.Label(panel, textvariable=status_var, style="Status.TLabel").pack(anchor="w", pady=(4, 10))
        button = ttk.Button(panel, text="Configure", command=configure)
        button.pack(anchor="w")
        return button

    def empty_results(self):
        for child in self.results_frame.winfo_children():
            child.destroy()
        ttk.Label(self.results_frame, text="Enter keys, then validate.", style="Panel.TLabel").grid(row=0, column=0, columnspan=2)

    def config_message(self) -> str:
        env_path = app_dir() / ENV_FILE_NAME
        endpoint = self.config.get("endpoint", "")
        token = self.config.get("status_token", "")
        if endpoint and token:
            return f"Loaded {ENV_FILE_NAME}: endpoint and status token configured."
        if endpoint:
            return f"Loaded {ENV_FILE_NAME}: endpoint configured, no status token."
        return f"Create {env_path} with SUB2API_ENDPOINT and optional STATUS_API_TOKEN."

    def client(self) -> KeyStatusClient:
        return KeyStatusClient(self.config.get("endpoint", ""), self.config.get("status_token", ""))

    def refresh_app_status(self):
        codex_installed, codex_location = detect_codex()
        opencode_installed, opencode_location = detect_opencode()
        self.codex_status.set(("Detected: " if codex_installed else "Not detected: ") + codex_location)
        self.opencode_status.set(("Detected: " if opencode_installed else "Not detected: ") + opencode_location)
        self.codex_button.configure(text="Configure" if codex_installed else "Download", command=self.configure_codex if codex_installed else self.download_codex)
        self.opencode_button.configure(text="Configure" if opencode_installed else "Download", command=self.configure_opencode if opencode_installed else self.download_opencode)

    def load_existing_keys_and_validate(self):
        openai_key, gemini_key = load_existing_keys()
        loaded = False
        if openai_key and not self.openai_key.get().strip():
            self.openai_key.insert(0, openai_key)
            loaded = True
        if gemini_key and not self.gemini_key.get().strip():
            self.gemini_key.insert(0, gemini_key)
            loaded = True
        if loaded and self.config.get("endpoint"):
            self.validate_keys()

    def download_codex(self):
        messagebox.showwarning("Configure after install", "After installing Codex, reopen this app and configure it.")
        open_url(CODEX_DOWNLOAD_URL)

    def download_opencode(self):
        messagebox.showwarning("Configure after install", "After installing OpenCode, reopen this app and configure it.")
        open_url(OPENCODE_DOWNLOAD_URL)

    def configure_codex(self):
        if not self.openai_key.get().strip():
            self.status.set("Enter an OpenAI / Codex key first.")
            return
        endpoint = self.config.get("endpoint", "")
        if not endpoint:
            self.status.set("Set the Sub2API endpoint first.")
            return
        backups = [item for item in (backup_if_exists(AppPaths.codex_config), backup_if_exists(AppPaths.codex_auth)) if item]
        write_codex_config(endpoint)
        write_codex_auth(self.openai_key.get().strip())
        self.status.set("Codex configured." + (f" Backups: {len(backups)}." if backups else ""))
        messagebox.showwarning("Restart required", "Restart or reset Codex so it reloads the new configuration.")

    def configure_opencode(self):
        endpoint = self.config.get("endpoint", "")
        if not endpoint:
            self.status.set("Set the Sub2API endpoint first.")
            return
        openai_key = self.openai_key.get().strip()
        gemini_key = self.gemini_key.get().strip()
        if not openai_key and not gemini_key:
            self.status.set("Enter at least one API key first.")
            return
        backups = [item for item in (backup_if_exists(AppPaths.opencode_config),) if item]
        write_opencode_config(openai_key, gemini_key, endpoint)
        self.status.set("OpenCode configured." + (f" Backups: {len(backups)}." if backups else ""))
        messagebox.showwarning("Restart required", "Restart or reset OpenCode so it reloads the new configuration.")

    def validate_keys(self):
        pairs = [("OpenAI / Codex", self.openai_key.get().strip()), ("Gemini", self.gemini_key.get().strip())]
        pairs = [(category, key) for category, key in pairs if key]
        if not pairs:
            self.status.set("Enter at least one key.")
            return
        self.validate_button.configure(state="disabled")
        self.status.set(f"Validating {len(pairs)} key(s)...")
        self.empty_results()
        threading.Thread(target=self._validate_worker, args=(pairs,), daemon=True).start()

    def _validate_worker(self, pairs: list[tuple[str, str]]):
        client = self.client()
        results = []
        for category, key in pairs:
            try:
                data = client.lookup(key)
                group = (data.get("group") or {}).get("name") or ""
                expected = "openai" if category == "OpenAI / Codex" else "gravity"
                type_ok = group.lower().startswith(expected)
                state = data.get("state") or {}
                results.append(KeyResult(
                    category=category,
                    key=key,
                    valid=type_ok,
                    title=data.get("name") or preview_key(key),
                    status=("Usable" if state.get("usable") else "Not usable") if type_ok else "Wrong type",
                    group=group,
                    expires_at=data.get("expires_at"),
                    charged=money(data.get("charged_usd")),
                    consumed=money(data.get("consumed_usd")),
                    remaining=money(data.get("remaining_usd")),
                    usable=bool(state.get("usable")) and type_ok,
                ))
            except urllib.error.HTTPError as exc:
                results.append(KeyResult(category, key, False, preview_key(key), "Invalid" if exc.code == 404 else f"HTTP {exc.code}"))
            except Exception as exc:
                results.append(KeyResult(category, key, False, preview_key(key), "Error"))
        self.after(0, self.render_results, results)

    def render_results(self, results: list[KeyResult]):
        for child in self.results_frame.winfo_children():
            child.destroy()
        for index, result in enumerate(results):
            self.result_card(result, index)
        self.status.set(f"Done. {sum(1 for r in results if r.valid)}/{len(results)} found, {sum(1 for r in results if r.usable)} usable.")
        self.validate_button.configure(state="normal")

    def result_card(self, result: KeyResult, index: int):
        card = ttk.Frame(self.results_frame, style="Panel.TFrame", padding=14)
        card.grid(row=0, column=index, sticky="nsew", padx=(0, 8) if index == 0 else (8, 0))
        chart = RingChart(card)
        chart.pack(side="right", padx=(14, 0))
        chart.draw(result.consumed, result.remaining, result.valid)
        top = ttk.Frame(card, style="Panel.TFrame")
        top.pack(fill="x")
        ttk.Label(top, text=result.title, style="CardTitle.TLabel").pack(side="left")
        color = GREEN if result.usable else (AMBER if result.valid else RED)
        badge = Canvas(top, width=112, height=28, bg=PANEL, highlightthickness=0)
        badge.pack(side="right")
        badge.create_rectangle(1, 3, 110, 25, fill="#0b1222", outline=color, width=1)
        badge.create_text(56, 14, text=result.status, fill=color, font=("Segoe UI", 9, "bold"))
        ttk.Label(card, text=f"{result.category}   {preview_key(result.key)}", style="Status.TLabel").pack(anchor="w", pady=(4, 12))
        fields = [
            ("Expires", format_local_datetime(result.expires_at) or "Never"),
            ("Charged", f"${result.charged:,.4f}"),
            ("Consumed", f"${result.consumed:,.4f}"),
            ("Remaining", f"${result.remaining:,.4f}"),
        ]
        for label, value in fields:
            ttk.Label(card, text=label, style="Status.TLabel").pack(anchor="w")
            ttk.Label(card, text=value, style="Panel.TLabel").pack(anchor="w", pady=(0, 7))


if __name__ == "__main__":
    App().mainloop()
