import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart' show DailyPerson;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// FaceStack renders a row of overlapping circular avatars for the people
/// behind an aggregated Home row (e.g. the helpers who pitched in on a request).
/// Shows at most [max] faces; callers convey any overflow in adjacent copy.
class FaceStack extends StatelessWidget {
  final List<DailyPerson> people;
  final double size;
  final int max;

  const FaceStack({
    super.key,
    required this.people,
    this.size = 20,
    this.max = 3,
  });

  @override
  Widget build(BuildContext context) {
    if (people.isEmpty) return const SizedBox.shrink();
    final shown = people.take(max).toList();
    final overlap = size * 0.35;
    return SizedBox(
      height: size,
      width: size + (shown.length - 1) * (size - overlap),
      child: Stack(
        children: [
          for (var i = 0; i < shown.length; i++)
            Positioned(
              left: i * (size - overlap),
              child: Container(
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: AppColors.cardBackground(context),
                    width: 2,
                  ),
                ),
                child: UserAvatar(
                  user: User(
                    id: shown[i].userId,
                    name: shown[i].displayName,
                    mediaId: shown[i].mediaId,
                  ),
                  radius: size / 2,
                ),
              ),
            ),
        ],
      ),
    );
  }
}
