"use client";

import { useMemo, useState } from "react";
import { Loader2, Plus } from "lucide-react";
import { toast } from "sonner";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import type { SkillSet, SkillSummary } from "@multica/core/types";
import {
  skillListOptions,
  skillSetDetailOptions,
  skillSetListOptions,
} from "@multica/core/workspace/queries";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SkillPickerList } from "../../agents/components/skill-picker-list";
import { useT } from "../../i18n";

interface CreateSkillSetDialogProps {
  onClose: () => void;
  onCreated?: (skillSet: SkillSet) => void;
}

export function CreateSkillSetDialog({
  onClose,
  onCreated,
}: CreateSkillSetDialogProps) {
  const { t } = useT("skills");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const { data: skills = [], isLoading } = useQuery(skillListOptions(wsId));
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set());
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const selected = useMemo(
    () => Array.from(selectedIds),
    [selectedIds],
  );

  const toggleSkill = (skill: SkillSummary) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(skill.id)) next.delete(skill.id);
      else next.add(skill.id);
      return next;
    });
  };

  const submit = async () => {
    const trimmed = name.trim();
    if (!trimmed || submitting) return;
    setSubmitting(true);
    setError("");
    try {
      const skillSet = await api.createSkillSet({
        name: trimmed,
        description: description.trim(),
        skill_ids: selected,
      });
      qc.setQueryData(skillSetDetailOptions(wsId, skillSet.id).queryKey, skillSet);
      qc.invalidateQueries({ queryKey: skillSetListOptions(wsId).queryKey });
      toast.success(t(($) => $.skill_sets.create.toast_created));
      onCreated?.(skillSet);
      onClose();
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t(($) => $.skill_sets.create.fallback_error),
      );
      setSubmitting(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[calc(100vh-2rem)] gap-0 overflow-hidden p-0 sm:max-w-lg">
        <DialogHeader className="border-b px-5 py-4">
          <DialogTitle>{t(($) => $.skill_sets.create.title)}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 overflow-y-auto px-5 py-4">
          <div className="space-y-1.5">
            <Label htmlFor="create-skill-set-name" className="text-xs text-muted-foreground">
              {t(($) => $.skill_sets.create.name_label)}
            </Label>
            <Input
              id="create-skill-set-name"
              autoFocus
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                setError("");
              }}
              placeholder={t(($) => $.skill_sets.create.name_placeholder)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="create-skill-set-description" className="text-xs text-muted-foreground">
              {t(($) => $.skill_sets.create.description_label)}
            </Label>
            <Textarea
              id="create-skill-set-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t(($) => $.skill_sets.create.description_placeholder)}
              rows={3}
            />
          </div>

          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">
              {t(($) => $.skill_sets.create.skills_label)}
            </Label>
            <SkillPickerList
              skills={skills}
              selectedIds={selectedIds}
              onToggle={toggleSkill}
              loading={isLoading}
              emptyMessage={t(($) => $.skill_sets.create.skills_empty)}
              noMatchMessage={t(($) => $.skill_sets.create.skills_no_match)}
            />
          </div>

          {error && (
            <div className="rounded-md border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {error}
            </div>
          )}
        </div>

        <DialogFooter className="rounded-none">
          <Button type="button" variant="outline" onClick={onClose} disabled={submitting}>
            {t(($) => $.skill_sets.create.cancel)}
          </Button>
          <Button type="button" onClick={submit} disabled={!name.trim() || submitting}>
            {submitting ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Plus className="h-3.5 w-3.5" />
            )}
            {submitting
              ? t(($) => $.skill_sets.create.creating)
              : t(($) => $.skill_sets.create.create)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
