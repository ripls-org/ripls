import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/services/providers.dart';
import '../../widgets/adaptive/auth_screen_wrapper.dart';
import '../../widgets/keyboard_dismiss_wrapper.dart';

/// ForgotPasswordScreen allows users to request a password reset email.
class ForgotPasswordScreen extends ConsumerStatefulWidget {
  const ForgotPasswordScreen({super.key});

  @override
  ConsumerState<ForgotPasswordScreen> createState() =>
      _ForgotPasswordScreenState();
}

class _ForgotPasswordScreenState extends ConsumerState<ForgotPasswordScreen> {
  final _formKey = GlobalKey<FormState>();
  final _emailController = TextEditingController();
  bool _isLoading = false;
  bool _emailSent = false;
  String? _errorMessage;

  @override
  void dispose() {
    _emailController.dispose();
    super.dispose();
  }

  Future<void> _handleSubmit() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final authService = ref.read(authServiceProvider);
      final result = await authService.requestPasswordReset(
        email: _emailController.text.trim(),
      );

      if (!mounted) return;

      if (result.success) {
        setState(() {
          _emailSent = true;
        });
      } else {
        setState(() {
          _errorMessage = result.error ?? 'Failed to send reset email';
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          _isLoading = false;
        });
      }
    }
  }

  Widget _buildEmailField() {
    return TextFormField(
      controller: _emailController,
      decoration: InputDecoration(
        labelText: context.l10n.loginEmailLabel,
        hintText: context.l10n.loginEmailHint,
        border: const OutlineInputBorder(),
        prefixIcon: const Icon(Icons.email),
      ),
      keyboardType: TextInputType.emailAddress,
      textInputAction: TextInputAction.done,
      validator: (value) {
        if (value == null || value.trim().isEmpty) {
          return context.l10n.loginEmailValidationEmpty;
        }
        if (!value.contains('@')) {
          return context.l10n.loginEmailValidationInvalid;
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
        onPressed: _isLoading ? null : _handleSubmit,
        child: _isLoading
            ? const SizedBox(
                height: 20,
                width: 20,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : Text(context.l10n.forgotPasswordButton),
      ),
    );
  }

  Widget _buildBackToLoginButton() {
    return TextButton(
      onPressed: () => context.go('/login'),
      child: const Text('Back to Login'),
    );
  }

  Widget _buildSuccessView() {
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Icon(
          Icons.check_circle_outline,
          size: 64,
          color: Theme.of(context).colorScheme.primary,
        ),
        const SizedBox(height: 24),
        Text(
          context.l10n.forgotPasswordSuccess,
          style: Theme.of(context).textTheme.headlineMedium,
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 16),
        Text(
          'We\'ve sent a password reset link to ${_emailController.text.trim()}. '
          'Please check your email and follow the instructions.',
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
            context.l10n.forgotPasswordScreenTitle,
            style: Theme.of(context).textTheme.headlineMedium,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
          Text(
            context.l10n.forgotPasswordInstructions,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodyMedium,
          ),
          const SizedBox(height: 32),
          _buildEmailField(),
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
        appBar: AppBar(title: Text(context.l10n.forgotPasswordScreenTitle)),
        body: KeyboardDismissWrapper(
          child: SafeArea(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: _emailSent ? _buildSuccessView() : _buildForm(),
            ),
          ),
        ),
      ),
    );
  }
}
