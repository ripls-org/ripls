import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// ImpactEditChip renders an inline numeric stepper with a tappable text
/// field, − / + buttons, and an optional unit suffix.
class ImpactEditChip extends StatefulWidget {
  const ImpactEditChip({
    super.key,
    required this.value,
    required this.onChanged,
    this.prefix,
    this.suffix,
    this.width = 56,
    this.min,
    this.max,
    this.step = 1,
  });

  final double value;
  final ValueChanged<double> onChanged;
  final String? prefix;
  final String? suffix;
  final double width;
  final double? min;
  final double? max;
  final double step;

  @override
  State<ImpactEditChip> createState() => _ImpactEditChipState();
}

class _ImpactEditChipState extends State<ImpactEditChip> {
  late final TextEditingController _controller;
  late final FocusNode _focus;
  bool _hasFocus = false;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController(text: _format(widget.value));
    _focus = FocusNode()
      ..addListener(() => setState(() => _hasFocus = _focus.hasFocus));
  }

  @override
  void didUpdateWidget(ImpactEditChip old) {
    super.didUpdateWidget(old);
    if (old.value != widget.value && !_hasFocus) {
      _controller.text = _format(widget.value);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    _focus.dispose();
    super.dispose();
  }

  String _format(double v) =>
      v == v.truncateToDouble() ? v.toInt().toString() : v.toStringAsFixed(1);

  double _clamp(double v) {
    if (widget.min != null && v < widget.min!) return widget.min!;
    if (widget.max != null && v > widget.max!) return widget.max!;
    return v;
  }

  bool get _atMin => widget.min != null && widget.value <= widget.min!;
  bool get _atMax => widget.max != null && widget.value >= widget.max!;

  void _bump(double delta) {
    final next = _clamp(widget.value + delta);
    if (next == widget.value) return;
    _controller.text = _format(next);
    widget.onChanged(next);
  }

  @override
  Widget build(BuildContext context) {
    final borderColor = _hasFocus
        ? ImpactModalColors.inputBorderFocus
        : ImpactModalColors.inputBorder;

    return Row(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        Container(
          decoration: BoxDecoration(
            color: ImpactModalColors.inputFieldBg,
            borderRadius: BorderRadius.circular(999),
            border: Border.all(
              color: borderColor,
              width: _hasFocus ? 1.5 : 1,
            ),
          ),
          height: 36,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              _StepButton(
                icon: Icons.remove_rounded,
                onTap: _atMin ? null : () => _bump(-widget.step),
              ),
              if (widget.prefix != null)
                Padding(
                  padding: const EdgeInsets.only(right: 1),
                  child: Text(
                    widget.prefix!,
                    style: const TextStyle(
                      fontSize: 15,
                      color: ImpactModalColors.ink,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              SizedBox(
                width: widget.width,
                child: TextField(
                  controller: _controller,
                  focusNode: _focus,
                  keyboardType:
                      const TextInputType.numberWithOptions(decimal: true),
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: ImpactModalColors.ink,
                  ),
                  decoration: const InputDecoration(
                    border: InputBorder.none,
                    isDense: true,
                    contentPadding: EdgeInsets.zero,
                  ),
                  onChanged: (v) {
                    final parsed = double.tryParse(v);
                    if (parsed == null) return;
                    widget.onChanged(_clamp(parsed));
                  },
                ),
              ),
              _StepButton(
                icon: Icons.add_rounded,
                onTap: _atMax ? null : () => _bump(widget.step),
              ),
            ],
          ),
        ),
        if (widget.suffix != null) ...[
          const SizedBox(width: 8),
          Text(
            widget.suffix!,
            style: const TextStyle(
              fontSize: 14,
              color: ImpactModalColors.inkSoft,
              fontWeight: FontWeight.w400,
            ),
          ),
        ],
      ],
    );
  }
}

class _StepButton extends StatelessWidget {
  const _StepButton({required this.icon, required this.onTap});

  final IconData icon;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final disabled = onTap == null;
    return InkResponse(
      onTap: onTap,
      radius: 18,
      containedInkWell: true,
      customBorder: const CircleBorder(),
      child: SizedBox(
        width: 32,
        height: 36,
        child: Icon(
          icon,
          size: 16,
          color: disabled
              ? ImpactModalColors.inkFade.withValues(alpha: 0.4)
              : ImpactModalColors.ink,
        ),
      ),
    );
  }
}
