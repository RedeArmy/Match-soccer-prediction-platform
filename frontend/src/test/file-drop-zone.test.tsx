import { afterEach, describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, createEvent } from "@testing-library/react";
import React from "react";
import { FileDropZone, MAX_FILE_BYTES } from "@/components/deposit/FileDropZone";

vi.mock("lucide-react", () => ({
  ImageIcon: () => <svg data-testid="image-icon" />,
}));

const defaults = {
  file: null as File | null,
  preview: null as string | null,
  onFileSelect: vi.fn(),
  onError: vi.fn(),
  tooLargeMsg: "Archivo muy grande",
  clickLabel: "Haz clic aquí",
  typesLabel: "PNG, JPG, PDF",
  filePrefix: "Archivo:",
};

function zone(overrides: Partial<typeof defaults> = {}) {
  return render(<FileDropZone {...{ ...defaults, ...overrides }} />);
}

afterEach(() => vi.clearAllMocks());

describe("FileDropZone", () => {
  // ── Rendering ─────────────────────────────────────────────────────────────

  it("renders clickLabel", () => {
    zone();
    expect(screen.getByText("Haz clic aquí")).toBeInTheDocument();
  });

  it("renders typesLabel", () => {
    zone();
    expect(screen.getByText("PNG, JPG, PDF")).toBeInTheDocument();
  });

  it("renders ImageIcon when no preview is set", () => {
    zone();
    expect(screen.getByTestId("image-icon")).toBeInTheDocument();
  });

  it("renders dragLabel when provided", () => {
    zone({ dragLabel: "o arrastra aquí" });
    expect(screen.getByText("o arrastra aquí")).toBeInTheDocument();
  });

  it("omits dragLabel when not provided", () => {
    zone();
    expect(screen.queryByText("o arrastra aquí")).toBeNull();
  });

  it("renders a preview image instead of the icon when preview prop is set", () => {
    zone({ preview: "data:image/jpeg;base64,abc" });
    expect(screen.getByAltText("Vista previa")).toBeInTheDocument();
    expect(screen.queryByTestId("image-icon")).toBeNull();
  });

  it("renders file name and size when file prop is set", () => {
    const f = new File(["x"], "receipt.pdf", { type: "application/pdf" });
    Object.defineProperty(f, "size", { value: 2048 });
    zone({ file: f });
    expect(screen.getByText("receipt.pdf")).toBeInTheDocument();
    expect(screen.getByText(/2 KB/)).toBeInTheDocument();
  });

  // ── File selection via input ───────────────────────────────────────────────

  it("calls onError when the selected file exceeds MAX_FILE_BYTES", () => {
    const onError = vi.fn();
    const { container } = zone({ onError });
    const big = new File(["x"], "big.jpg", { type: "image/jpeg" });
    Object.defineProperty(big, "size", { value: MAX_FILE_BYTES + 1 });
    const input = container.querySelector('input[type="file"]')!;
    fireEvent.change(input, { target: { files: [big] } });
    expect(onError).toHaveBeenCalledWith("Archivo muy grande");
  });

  it("calls onFileSelect with null preview for non-image files", () => {
    const onFileSelect = vi.fn();
    const { container } = zone({ onFileSelect });
    const pdf = new File(["pdf"], "doc.pdf", { type: "application/pdf" });
    const input = container.querySelector('input[type="file"]')!;
    fireEvent.change(input, { target: { files: [pdf] } });
    expect(onFileSelect).toHaveBeenCalledWith(pdf, null);
  });

  it("calls onFileSelect with data URL preview for image files", () => {
    const FAKE_URL = "data:image/jpeg;base64,test==";

    // Use a class so `new FileReader()` works (arrow functions can't be constructors).
    class FakeReader {
      result = FAKE_URL;
      onload: (() => void) | null = null;
      readAsDataURL() {
        this.onload?.();
      }
    }
    vi.stubGlobal("FileReader", FakeReader);

    const onFileSelect = vi.fn();
    const { container } = zone({ onFileSelect });
    const img = new File(["img"], "photo.jpg", { type: "image/jpeg" });
    fireEvent.change(container.querySelector('input[type="file"]')!, {
      target: { files: [img] },
    });

    expect(onFileSelect).toHaveBeenCalledWith(img, FAKE_URL);
    vi.unstubAllGlobals();
  });

  // ── Click / keyboard ──────────────────────────────────────────────────────

  it("triggers file input click when the drop zone is clicked", () => {
    const clickSpy = vi
      .spyOn(HTMLInputElement.prototype, "click")
      .mockImplementation(() => {});
    const { container } = zone();
    fireEvent.click(container.querySelector('[role="button"]')!);
    expect(clickSpy).toHaveBeenCalledOnce();
    clickSpy.mockRestore();
  });

  it("triggers file input click on Enter keydown", () => {
    const clickSpy = vi
      .spyOn(HTMLInputElement.prototype, "click")
      .mockImplementation(() => {});
    const { container } = zone();
    fireEvent.keyDown(container.querySelector('[role="button"]')!, { key: "Enter" });
    expect(clickSpy).toHaveBeenCalledOnce();
    clickSpy.mockRestore();
  });

  it("triggers file input click on Space keydown", () => {
    const clickSpy = vi
      .spyOn(HTMLInputElement.prototype, "click")
      .mockImplementation(() => {});
    const { container } = zone();
    fireEvent.keyDown(container.querySelector('[role="button"]')!, { key: " " });
    expect(clickSpy).toHaveBeenCalledOnce();
    clickSpy.mockRestore();
  });

  it("does not trigger file input click for other keys", () => {
    const clickSpy = vi
      .spyOn(HTMLInputElement.prototype, "click")
      .mockImplementation(() => {});
    const { container } = zone();
    fireEvent.keyDown(container.querySelector('[role="button"]')!, { key: "Tab" });
    expect(clickSpy).not.toHaveBeenCalled();
    clickSpy.mockRestore();
  });

  // ── Drag and drop ─────────────────────────────────────────────────────────

  it("calls preventDefault on dragOver", () => {
    const { container } = zone();
    const dropZone = container.querySelector('[role="button"]')!;
    const dragOverEvent = createEvent.dragOver(dropZone);
    const preventDefaultSpy = vi.spyOn(dragOverEvent, "preventDefault");
    fireEvent(dropZone, dragOverEvent);
    expect(preventDefaultSpy).toHaveBeenCalledOnce();
  });

  it("calls onFileSelect when a file is dropped on the zone", () => {
    const onFileSelect = vi.fn();
    const { container } = zone({ onFileSelect });
    const dropZone = container.querySelector('[role="button"]')!;
    const pdf = new File(["content"], "proof.pdf", { type: "application/pdf" });

    const dropEvent = createEvent.drop(dropZone);
    Object.defineProperty(dropEvent, "dataTransfer", {
      value: { files: [pdf] },
      configurable: true,
    });
    fireEvent(dropZone, dropEvent);

    expect(onFileSelect).toHaveBeenCalledWith(pdf, null);
  });
});
