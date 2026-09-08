import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';

/// RequestEditPane renders the editable title + description fields for a
/// request in edit mode. Read mode lives in `RequestReadShell`; this surface is
/// shown beneath the top edit bar (Cancel / Save) when `state.isEditing`.
class RequestEditPane extends StatelessWidget {
  final String? editingTitle;
  final String? editingDescription;
  final ValueChanged<String>? onTitleChanged;
  final ValueChanged<String>? onDescriptionChanged;

  const RequestEditPane({
    super.key,
    this.editingTitle,
    this.editingDescription,
    this.onTitleChanged,
    this.onDescriptionChanged,
  });

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          ContentViewBuilders.buildEditableTitle(
            value: editingTitle ?? '',
            onChanged: onTitleChanged ?? (_) {},
            enabled: true,
            label: 'Request Title',
            hintText: 'Request title',
          ),
          const SizedBox(height: 8),
          ContentViewBuilders.buildEditableDescription(
            value: editingDescription ?? '',
            onChanged: onDescriptionChanged ?? (_) {},
            enabled: true,
            label: 'Description',
            hintText: 'Request description',
          ),
        ],
      ),
    );
  }
}
