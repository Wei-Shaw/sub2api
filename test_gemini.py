from google import genai
from google.genai import types

client = genai.Client(
    api_key="sk-d0678689ab0000696efa7fc0679026bf7bedb954d98ef5da9aa8161f99227ef8",
    http_options={"api_version": "v1beta", "base_url": "https://clicodeplus.com"}
)

try:
    response = client.models.generate_content(
        model="gemini-3.1-flash-image-preview",
        contents="Draw a red circle",
        config=types.GenerateContentConfig(
            response_modalities=["TEXT", "IMAGE"]
        )
    )
    print("Response received!")
    for part in response.candidates[0].content.parts:
        if part.text:
            print("Text:", part.text[:200])
        if part.inline_data:
            print("Image: mime=%s, size=%d bytes" % (part.inline_data.mime_type, len(part.inline_data.data)))
    um = response.usage_metadata
    if um:
        print("Prompt tokens:", um.prompt_token_count)
        print("Candidates tokens:", um.candidates_token_count)
        print("Total tokens:", um.total_token_count)
        if hasattr(um, "candidates_tokens_details") and um.candidates_tokens_details:
            for d in um.candidates_tokens_details:
                print("  Detail: modality=%s tokens=%d" % (d.modality, d.token_count))
except Exception as e:
    print("Error:", type(e).__name__, e)
