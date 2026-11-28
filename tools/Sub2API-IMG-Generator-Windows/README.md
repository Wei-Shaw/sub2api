# Sub2API Image Generator

Small native Windows app for generating images through a `sub2api` OpenAI group.

## Native Windows App

Run:

```text
native-dist\Sub2ApiImageGenerator.exe
```

This is the native Windows Forms version. It does not bundle Chromium or Node.

Current behavior:

- Lets the user enter any OpenAI-compatible Sub2API base URL, such as `https://your-host.example/v1` or `http://192.168.1.10:8000/v1`.
- Adds `/v1` automatically when the user enters only a host, and defaults local/IP hosts to `http://` when no scheme is provided.
- Saves the entered base URL in `base-url.txt` next to the exe.
- Saves the entered API key in `api-key.txt` next to the exe.
- Saves generated images into `GeneratedImages` next to the exe.
- Loads previous images from `GeneratedImages` as history on startup.
- Includes a cancel button while generation is running.
- Supports Low, Medium, and High effort image generation modes. Low is the default for faster responses.
- Supports 1K, 2K, and 4K output sizes. Sub2API uses the size to classify image billing as 1K, 2K, or 4K.
- Supports selecting a reference image from the PC, previewing it, and sending it with the prompt as image context.
- Switches the prompt field to RTL when the first word is Persian or Arabic, while preserving mixed English words in the sentence.
