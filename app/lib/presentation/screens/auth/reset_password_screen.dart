import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/services/providers.dart';

import '../../widgets/adaptive/auth_screen_wrapper.dart';
import '../../widgets/keyboard_dismiss_wrapper.dart';

/// ResetPasswordScreen allows users to set a new password using a reset token.
class ResetPasswordScreen extends ConsumerStatefulWidget {
  final String token;

  const ResetPasswordScreen({super.key, required this.token});

  @override
  ConsumerState<ResetPasswordScreen> createState() =>
      _ResetPasswordScreenState();
}

class _ResetPasswordScreenState extends ConsumerState<ResetPasswordScreen> {
  final _formKey = GlobalKey<FormState>();
  final _passwordController = TextEditingController();
  final _confirmPasswordController = TextEditingController();
  bool _isLoading = true;
  bool _isSubmitting = false;
  bool _tokenValid = false;
  bool _obscurePassword = true;
  bool _obscureConfirmPassword = true;
  String? _email;
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    _checkToken();
  }

  @override
  void dispose() {
    _passwordController.dispose();
    _confirmPasswordController.dispose();
    super.dispose();
  }

  Future<void> _checkToken() async {
    final authService = ref.read(authServiceProvider);
    final result = await authService.checkResetPasswordToken(
      token: widget.token,
    );

    if (!mounted) return;

    setState(() {
      _isLoading = false;
      _tokenValid = result.isValid;
      _email = result.email;
      _errorMessage = result.isValid ? null : result.errorMessage;
    });
  }

  Future<void> _handleSubmit() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }

    setState(() {
      _isSubmitting = true;
      _errorMessage = null;
    });

    try {
      final authService = ref.read(authServiceProvider);
      final result = await authService.resetPassword(
        token: widget.token,
        newPassword: _passwordController.text,
      );

      if (!mounted) return;

      if (result.success) {
        // Show success message
        ToastHelper.showSuccess(context, 'Password reset successful! Please log in.');

        // Navigate to login
        context.go('/login');
      } else {
        setState(() {
          _errorMessage = result.error ?? 'Failed to reset password';
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          _isSubmitting = false;
        });
      }
    }
  }

  Widget _buildPasswordField() {
    return TextFormField(
      controller: _passwordController,
      decoration: InputDecoration(
        labelText: context.l10n.resetPasswordNewLabel,
        border: const OutlineInputBorder(),
        prefixIcon: const Icon(Icons.lock),
        suffixIcon: IconAction(
          icon: _obscurePassword ? Icons.visibility : Icons.visibility_off,
          semanticsLabel: _obscurePassword
              ? context.l10n.a11yShowPassword
              : context.l10n.a11yHidePassword,
          onPressed: () {
            setState(() {
              _obscurePassword = !_obscurePassword;
            });
          },
        ),
      ),
      obscureText: _obscurePassword,
      textInputAction: TextInputAction.next,
      validator: (value) {
        if (value == null || value.isEmpty) {
          return context.l10n.loginPasswordValidationEmpty;
        }
        if (value.length < 8) {
          return context.l10n.loginPasswordValidationShort;
        }
        return null;
      },
    );
  }

  Widget _buildConfirmPasswordField() {
    return TextFormField(
      controller: _confirmPasswordController,
      decoration: InputDecoration(
        labelText: context.l10n.resetPasswordConfirmLabel,
        border: const OutlineInputBorder(),
        prefixIcon: const Icon(Icons.lock),
        suffixIcon: IconAction(
          icon: _obscureConfirmPassword ? Icons.visibility : Icons.visibility_off,
          semanticsLabel: _obscureConfirmPassword
              ? context.l10n.a11yShowPassword
              : context.l10n.a11yHidePassword,
          onPressed: () {
            setState(() {
              _obscureConfirmPassword = !_obscureConfirmPassword;
            });
          },
        ),
      ),
      obscureText: _obscureConfirmPassword,
      textInputAction: TextInputAction.done,
      validator: (value) {
        if (value == null || value.isEmpty) {
          return context.l10n.loginPasswordValidationEmpty;
        }
        if (value != _passwordController.text) {
          return context.l10n.resetPasswordMismatch;
        }
        return null;
      },
      onFieldSubmitted: (_) => _handleSubmit(),
    );
  }

  Widget _buildErrorMessage(BuildContext context) {
    if (_errorMessage == null) return const SizedBox.shrink();

    return Padding(
      padding: const EdgeInsets.only(bottom: 16),
      child: Text(
        _errorMessage!,
        style: TextStyle(color: Theme.of(context).colorScheme.error),
        textAlign: TextAlign.center,
      ),
    );
  }

  Widget _buildSubmitButton() {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton(
        onPressed: _isSubmitting ? null : _handleSubmit,
        child: _isSubmitting
            ? const SizedBox(
                height: 20,
                width: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : Text(context.l10n.resetPasswordButton),
      ),
    );
  }

  Widget _buildBackToLoginButton() {
    return TextButton(
      onPressed: () => context.go('/login'),
      child: const Text('Back to Login'),
    );
  }

  Widget _buildLoadingView() {
    return const Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          CircularProgressIndicator(),
          SizedBox(height: 16),
          Text('Validating reset link...'),
        ],
      ),
    );
  }

  Widget _buildInvalidTokenView() {
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Icon(
          Icons.error_outline,
          size: 64,
          color: Theme.of(context).colorScheme.error,
        ),
        const SizedBox(height: 24),
        Text(
          'Invalid or Expired Link',
          style: Theme.of(context).textTheme.headlineMedium,
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 16),
        Text(
          _errorMessage ?? 'This password reset link is invalid or has expired.',
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.bodyMedium,
        ),
        const SizedBox(height: 32),
        _buildBackToLoginButton(),
      ],
    );
  }

  Widget _buildForm() {
    return Form(
      key: _formKey,
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            context.l10n.resetPasswordScreenTitle,
            style: Theme.of(context).textTheme.headlineMedium,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
          if (_email != null) ...[
            Text(
              'Resetting password for $_email',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            const SizedBox(height: 24),
          ],
          _buildPasswordField(),
          const SizedBox(height: 16),
          _buildConfirmPasswordField(),
          const SizedBox(height: 24),
          _buildErrorMessage(context),
          _buildSubmitButton(),
          const SizedBox(height: 16),
          _buildBackToLoginButton(),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AuthScreenWrapper(
      child: Scaffold(
        appBar: AppBar(title: Text(context.l10n.resetPasswordScreenTitle)),
        body: KeyboardDismissWrapper(
          child: SafeArea(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: _isLoading
                  ? _buildLoadingView()
                  : _tokenValid
                      ? _buildForm()
                      : _buildInvalidTokenView(),
            ),
          ),
        ),
      ),
    );
  }
}
