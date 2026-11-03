# Third-Party Notices

This file records third-party components added for the optional Unlimited OCR integration.
It is not a substitute for legal review.

## Baidu Unlimited OCR

- Upstream: `baidu/Unlimited-OCR` and `baidu/Unlimited-OCR-2`
- License reported by the upstream Hugging Face model metadata and GitHub LICENSE: MIT.
- The model repositories contain custom modeling code loaded with `trust_remote_code=True`.
  Pin a reviewed revision before deployment.
- Upstream acknowledges the DeepSeek-OCR/DeepSeek-OCR-2 and PaddleOCR projects. Review those
  notices and their licenses before redistributing a bundled image.

## Python / container dependencies

The sidecar image pins PyTorch, torchvision, and Transformers versions in
`ocr-vlm/Dockerfile`; FastAPI, Pillow, PyMuPDF, and supporting packages retain their own
licenses. Generate an SBOM from the built image and review it before release.
