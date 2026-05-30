import re

def extract(fn, indent_key=4):
    lines = open(fn, encoding="utf-8").read().split("\n")
    pat = re.compile(r"^" + (" " * indent_key) + r"riskControl: \{")
    closepat = re.compile(r"^" + (" " * indent_key) + r"\},?\s*$")
    start = next(i for i, l in enumerate(lines) if pat.match(l))
    end = next(j for j in range(start + 1, len(lines)) if closepat.match(lines[j]))
    # body excludes the `riskControl: {` opener and the closing `},`
    body = lines[start + 1:end]
    # strip 6 spaces of leading indent (keys were nested under admin.riskControl)
    dedented = []
    for l in body:
        if l.startswith("      "):
            dedented.append(l[6:])
        else:
            dedented.append(l.lstrip())
    return "\n".join(dedented)

for fn, lang in [("../../../frontend/src/i18n/locales/en.ts", "en"),
                 ("../../../frontend/src/i18n/locales/zh.ts", "zh")]:
    body = extract(fn)
    content = (
        "// Content Moderation (Risk Control) i18n messages.\n"
        "// Extracted from the core frontend (frontend/src/i18n/locales/%s.ts,\n"
        "// admin.riskControl block). registerNamespace deep-merges these under the\n"
        "// `admin.riskControl.*` keys, so the migrated view keeps its t() calls.\n"
        "export default {\n"
        "  admin: {\n"
        "    riskControl: {\n" % lang
    )
    # re-indent body to sit under admin.riskControl (6 spaces)
    indented = "\n".join(("      " + l) if l.strip() else l for l in body.split("\n"))
    content += indented + "\n"
    content += "    },\n  },\n}\n"
    out = "src/i18n/%s.ts" % lang
    open(out, "w", encoding="utf-8", newline="\n").write(content)
    print("wrote", out, "lines", content.count(chr(10)))
