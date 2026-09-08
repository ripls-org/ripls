import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/impact/formula_component_card.dart';
import 'package:ripls/presentation/widgets/impact/formula_visualization.dart';
import 'package:ripls/presentation/widgets/impact/reference_list.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// MetricDrillDownScreen shows a formula-based breakdown of a metric.
///
/// Three sections:
/// 1. Formula Banner — top-level formula with real values
/// 2. Component Breakdown — recursive tree of formula components
/// 3. Research References — numbered citations
class MetricDrillDownScreen extends StatefulWidget {
  final DrillDownData data;

  const MetricDrillDownScreen({
    super.key,
    required this.data,
  });

  /// Pushes this screen onto the navigation stack.
  static void push(BuildContext context, {required DrillDownData data}) {
    Navigator.of(context).push(
      PageRouteBuilder(
        pageBuilder: (context, animation, secondaryAnimation) =>
            MetricDrillDownScreen(data: data),
        transitionsBuilder: (context, animation, secondaryAnimation, child) {
          return SlideTransition(
            position: Tween<Offset>(
              begin: const Offset(1, 0),
              end: Offset.zero,
            ).animate(CurvedAnimation(
              parent: animation,
              curve: Curves.easeInOut,
            )),
            child: child,
          );
        },
      ),
    );
  }

  @override
  State<MetricDrillDownScreen> createState() => _MetricDrillDownScreenState();
}

class _MetricDrillDownScreenState extends State<MetricDrillDownScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    return buildSwipeableScaffold(
      appBar: AppBar(
        backgroundColor: AppColors.surface(context),
        elevation: 0,
        centerTitle: false,
        leading: AppBarBackButton(onPressed: handleClose),
        title: Text(
          widget.data.metricName,
          style: TextStyle(
            color: AppColors.textPrimary(context),
            fontSize: 18,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
      backgroundColor: AppColors.surface(context),
      body: SingleChildScrollView(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _buildFormulaBanner(),
              const SizedBox(height: 24),
              _buildComponentBreakdown(),
              const SizedBox(height: 24),
              ReferenceList(references: widget.data.references),
              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    );
  }

  // ─── Section 1: Formula Banner ──────────────────────────────────────

  Widget _buildFormulaBanner() {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: AppColors.border(context),
          width: 1,
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildBannerHeader(),
          const SizedBox(height: 16),
          FormulaVisualization(formula: widget.data.formula),
        ],
      ),
    );
  }

  Widget _buildBannerHeader() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                widget.data.metricName.toUpperCase(),
                style: TextStyle(
                  color: AppColors.textSecondary(context),
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  letterSpacing: 0.5,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                widget.data.formattedValue,
                style: TextStyle(
                  color: AppColors.textPrimary(context),
                  fontSize: 32,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
        ),
        _buildConfidenceBadge(),
      ],
    );
  }

  Widget _buildConfidenceBadge() {
    final confidence = widget.data.confidence;
    String text;
    Color color;

    if (confidence >= 0.8) {
      text = 'Good confidence';
      color = PaperTokens.accentGreen;
    } else if (confidence >= 0.5) {
      text = 'Medium confidence';
      color = AppColors.statusWarning(context);
    } else {
      text = 'Low confidence';
      color = AppColors.statusError(context);
    }

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.check, size: 18, color: color),
        const SizedBox(width: 4),
        Text(
          text,
          style: TextStyle(
            color: color,
            fontSize: 14,
            fontWeight: FontWeight.w500,
          ),
        ),
      ],
    );
  }

  // ─── Section 2: Component Breakdown ─────────────────────────────────

  Widget _buildComponentBreakdown() {
    final formula = widget.data.formula;
    final hasOperands =
        formula.operands != null && formula.operands!.isNotEmpty;
    final components = hasOperands ? formula.operands! : <FormulaNode>[];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'HOW IT\'S CALCULATED',
          style: TextStyle(
            color: AppColors.textSecondary(context),
            fontSize: 11,
            fontWeight: FontWeight.w600,
            letterSpacing: 0.5,
          ),
        ),
        const SizedBox(height: 12),
        if (formula.explanation != null) ...[
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Text(
              formula.explanation!,
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 13,
                height: 1.4,
              ),
            ),
          ),
        ],
        ...components.map(
          (node) => FormulaComponentCard(node: node),
        ),
        // Show aggregate insight directly when there are no operand cards.
        if (!hasOperands && formula.aggregateInsight != null)
          FormulaComponentCard(
            node: FormulaNode(
              label: formula.label,
              formattedValue: formula.formattedValue,
              aggregateInsight: formula.aggregateInsight,
            ),
          ),
      ],
    );
  }
}
