"use client";

import { useMemo, useState } from "react";
import { BarChart3Icon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

interface UsageMetrics {
  totalCostUsd: number;
  totalMessagesSent: number;
  totalTokensUsed: number;
}

interface MonthOption {
  id: string;
  label: string;
}

interface UsageDataset {
  metricsByMonth: Record<string, UsageMetrics>;
  months: MonthOption[];
}

const monthLabelFormatter = new Intl.DateTimeFormat("en-US", {
  month: "long",
  year: "numeric",
});
const integerFormatter = new Intl.NumberFormat("en-US");
const usdFormatter = new Intl.NumberFormat("en-US", {
  currency: "USD",
  style: "currency",
});

function buildUsageDataset(): UsageDataset {
  const start = new Date(2026, 0, 1);
  const end = new Date();
  const months: MonthOption[] = [];
  const metricsByMonth: Record<string, UsageMetrics> = {};
  let index = 0;

  for (
    let cursor = new Date(start);
    cursor <= end;
    cursor = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 1)
  ) {
    const monthId = `${cursor.getFullYear()}-${String(
      cursor.getMonth() + 1,
    ).padStart(2, "0")}`;
    const totalMessagesSent = 480 + index * 95;
    const totalTokensUsed = 245_000 + index * 64_000;
    const totalCostUsd = Number(
      ((totalTokensUsed / 1_000_000) * 3.2 + index * 1.8 + 4.5).toFixed(2),
    );

    months.push({ id: monthId, label: monthLabelFormatter.format(cursor) });
    metricsByMonth[monthId] = {
      totalCostUsd,
      totalMessagesSent,
      totalTokensUsed,
    };
    index += 1;
  }

  return { metricsByMonth, months };
}

export function UsageSection() {
  const dataset = useMemo(() => buildUsageDataset(), []);
  const defaultMonthId = dataset.months.at(-1)?.id ?? "2026-01";
  const [selectedMonthId, setSelectedMonthId] = useState(defaultMonthId);
  const selectedUsage =
    dataset.metricsByMonth[selectedMonthId] ??
    dataset.metricsByMonth[defaultMonthId];
  const selectedMonthLabel =
    dataset.months.find((month) => month.id === selectedMonthId)?.label ??
    "Current month";

  return (
    <Card className="md:col-span-2">
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2 text-base">
            <BarChart3Icon className="size-4" />
            Usage
          </CardTitle>
          <p className="text-muted-foreground text-sm">
            Monthly usage and cost overview.
          </p>
        </div>
        <Badge variant="secondary">Demo data</Badge>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-muted-foreground text-sm">
            Showing stats for {selectedMonthLabel}.
          </p>
          <Select onValueChange={setSelectedMonthId} value={selectedMonthId}>
            <SelectTrigger className="w-full sm:w-56" size="sm">
              <SelectValue placeholder="Select month" />
            </SelectTrigger>
            <SelectContent align="end">
              {dataset.months.map((month) => (
                <SelectItem key={month.id} value={month.id}>
                  {month.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          <div className="rounded-lg border bg-card p-3">
            <p className="text-muted-foreground text-xs">Total messages sent</p>
            <p className="mt-1 font-semibold text-xl">
              {integerFormatter.format(selectedUsage.totalMessagesSent)}
            </p>
          </div>
          <div className="rounded-lg border bg-card p-3">
            <p className="text-muted-foreground text-xs">Total tokens used</p>
            <p className="mt-1 font-semibold text-xl">
              {integerFormatter.format(selectedUsage.totalTokensUsed)}
            </p>
          </div>
          <div className="rounded-lg border bg-card p-3">
            <p className="text-muted-foreground text-xs">
              Total cost for month
            </p>
            <p className="mt-1 font-semibold text-xl">
              {usdFormatter.format(selectedUsage.totalCostUsd)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
