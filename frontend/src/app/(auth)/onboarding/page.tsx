"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useQuery, useMutation } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { LoadingSpinner } from "@/components/shared/LoadingState";

export default function OnboardingPage() {
  const { getToken } = useAuth();
  const router = useRouter();
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState("");

  // Verify backend user exists — retry on 401 because the Clerk user.created
  // webhook may not yet have been delivered and processed.
  const { isLoading: checking } = useQuery({
    queryKey: ["onboarding-me"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMe(token!);
    },
    retry: (failureCount, error: unknown) => {
      const status = (error as { status?: number })?.status;
      // 401 = user.created webhook not yet processed; retry up to 5 times
      if (status === 401) return failureCount < 5;
      return false;
    },
    retryDelay: 2000,
  });

  const save = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      if (displayName.trim()) {
        await api.updateMe(token!, { display_name: displayName.trim() });
      }
    },
    onSuccess: () => router.replace("/dashboard"),
    onError: () => setError("Error guardando tu perfil. Intenta de nuevo."),
  });

  if (checking) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <LoadingSpinner size={32} />
      </div>
    );
  }

  return (
    <div className="max-w-md mx-auto py-12">
      <div className="card p-8 space-y-6">
        <div className="text-center">
          <h1 className="font-display text-3xl text-gold-400 mb-1">
            ¡BIENVENIDO!
          </h1>
          <p className="text-text-secondary text-sm">
            Cuéntanos un poco sobre ti
          </p>
        </div>

        <div className="space-y-4">
          <div>
            <label
              htmlFor="onboarding-display-name"
              className="block text-sm text-text-secondary mb-1.5"
            >
              Nombre para mostrar{" "}
              <span className="text-text-muted">(opcional)</span>
            </label>
            <input
              id="onboarding-display-name"
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Ej. Carlitos"
              maxLength={50}
              className="input-base"
            />
          </div>
        </div>

        {error && <p className="text-red-400 text-sm text-center">{error}</p>}

        <button
          onClick={() => save.mutate()}
          disabled={save.isPending}
          className="btn-gold w-full flex items-center justify-center gap-2"
        >
          {save.isPending ? (
            <LoadingSpinner size={18} />
          ) : (
            "Continuar al Dashboard"
          )}
        </button>

        <button
          onClick={() => router.replace("/dashboard")}
          className="block w-full text-center text-xs text-text-muted hover:text-text-secondary transition-colors"
        >
          Omitir por ahora
        </button>
      </div>
    </div>
  );
}
