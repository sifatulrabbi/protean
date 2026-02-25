"use client";

import { useState } from "react";
import { CircleDollarSignIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";

export function UsageSettingsPanel() {
  const [monthlyBudget, setMonthlyBudget] = useState("100");
  const [warningThreshold, setWarningThreshold] = useState("80");
  const [hardLimit, setHardLimit] = useState(true);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CircleDollarSignIcon className="size-5" />
          Usage Controls
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="space-y-2">
          <p className="font-medium text-sm">Monthly spending budget (USD)</p>
          <Input
            inputMode="decimal"
            min="0"
            onChange={(event) => setMonthlyBudget(event.target.value)}
            placeholder="Enter monthly budget"
            step="0.01"
            type="number"
            value={monthlyBudget}
          />
          <p className="text-muted-foreground text-xs">
            Limit total spend for this account each month.
          </p>
        </div>

        <div className="space-y-2">
          <p className="font-medium text-sm">Usage warning threshold</p>
          <Select onValueChange={setWarningThreshold} value={warningThreshold}>
            <SelectTrigger className="w-full sm:w-56">
              <SelectValue placeholder="Choose threshold" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="50">50% of budget</SelectItem>
              <SelectItem value="70">70% of budget</SelectItem>
              <SelectItem value="80">80% of budget</SelectItem>
              <SelectItem value="90">90% of budget</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            We can warn you when you get close to your monthly cap.
          </p>
        </div>

        <div className="flex items-start justify-between gap-3 rounded-lg border p-3">
          <div>
            <p className="font-medium text-sm">Enforce hard limit</p>
            <p className="text-muted-foreground text-xs">
              Stop requests automatically once budget is reached.
            </p>
          </div>
          <Switch checked={hardLimit} onCheckedChange={setHardLimit} />
        </div>

        <div className="flex justify-end">
          <Button type="button">Save usage settings</Button>
        </div>
      </CardContent>
    </Card>
  );
}
