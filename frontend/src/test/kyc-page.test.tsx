import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  getToken: vi.fn(),
  getKYCStatus: vi.fn(),
  getKYCDocuments: vi.fn(),
  uploadKYCDocument: vi.fn(),
  submitKYC: vi.fn(),
  deleteKYCDocument: vi.fn(),
}));

vi.mock("@clerk/nextjs", () => ({
  useAuth: () => ({ getToken: mocks.getToken }),
}));

vi.mock("@/hooks/useKYCStatus", () => ({
  useKYCStatus: () => mocks.getKYCStatus(),
}));

vi.mock("@/lib/api", () => ({
  api: {
    getKYCDocuments: mocks.getKYCDocuments,
    uploadKYCDocument: mocks.uploadKYCDocument,
    submitKYC: mocks.submitKYC,
    deleteKYCDocument: mocks.deleteKYCDocument,
  },
}));

import KYCPage from "@/app/(auth)/kyc/page";
import { I18nProvider } from "@/lib/i18n";

function renderKYCPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <I18nProvider>
      <QueryClientProvider client={queryClient}>
        <KYCPage />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

function makeJpegFile(name: string) {
  return new File([new Uint8Array([0xff, 0xd8, 0xff, 0xdb])], name, {
    type: "image/jpeg",
  });
}

describe("KYCPage document upload flow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getToken.mockResolvedValue("tok_kyc");
    mocks.getKYCStatus.mockReturnValue({
      data: { id: 1, user_id: "u1", status: "pending", tier: 1 },
      isLoading: false,
    });
    mocks.getKYCDocuments.mockResolvedValue([]);
    mocks.uploadKYCDocument
      .mockResolvedValueOnce({ id: 101, document_type: "gov_id", status: "pending" })
      .mockResolvedValueOnce({ id: 102, document_type: "selfie", status: "pending" });
    vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:preview");
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  });

  it("preloads selected documents locally and only persists them after clicking upload", async () => {
    const { container } = renderKYCPage();

    await screen.findByText("Documentos");
    const inputs = container.querySelectorAll<HTMLInputElement>('input[type="file"]');
    expect(inputs).toHaveLength(2);

    fireEvent.change(inputs[0], { target: { files: [makeJpegFile("id-front.jpg")] } });

    await screen.findByText("id-front.jpg");
    expect(screen.queryByRole("button", { name: /cargar archivos/i })).toBeNull();
    expect(mocks.uploadKYCDocument).not.toHaveBeenCalled();

    fireEvent.change(inputs[1], { target: { files: [makeJpegFile("selfie.jpg")] } });

    await screen.findByText("selfie.jpg");
    expect(mocks.uploadKYCDocument).not.toHaveBeenCalled();

    const uploadButton = screen.getByRole("button", { name: /cargar archivos/i });
    expect(uploadButton).toBeVisible();
    expect(uploadButton).toHaveClass("w-full");
    expect(uploadButton.previousElementSibling).toHaveClass("grid");

    fireEvent.click(uploadButton);

    await waitFor(() => expect(mocks.uploadKYCDocument).toHaveBeenCalledTimes(2));
    expect(mocks.uploadKYCDocument).toHaveBeenNthCalledWith(
      1,
      "tok_kyc",
      expect.any(FormData),
    );
    expect(mocks.uploadKYCDocument).toHaveBeenNthCalledWith(
      2,
      "tok_kyc",
      expect.any(FormData),
    );
  });
});
