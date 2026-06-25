"use client";

import { BankTransfersTab } from "@/components/admin/BankTransfersTab";
import { AdminPageHeader } from "@/components/admin/shared";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@clerk/nextjs";
import { api } from "@/lib/api";

export default function AdminBankTransfersPage() {
  const { getToken } = useAuth();
  const {
    data: transfers = [],
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["admin", "bank-transfers"],
    queryFn: async () => {
      const token = await getToken();
      return api.adminListBankTransfers(token!);
    },
  });

  return (
    <div className="space-y-6">
      <AdminPageHeader
        title="Transferencias Bancarias"
        subtitle={`${transfers.length} comprobantes en total`}
        onRefresh={refetch}
        isLoading={isLoading}
      />
      <BankTransfersTab />
    </div>
  );
}
